// Package remotedata 提供已授權 P2P 工作階段的檔案查詢與受限輸出。
package remotedata

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
	"yourdesk/internal/p2p"
)

type Search struct {
	Path    string `json:"path,omitempty"`
	Query   string `json:"query,omitempty"`
	Content string `json:"content,omitempty"`
	Limit   int    `json:"limit,omitempty"`
	Depth   int    `json:"depth,omitempty"`
}
type Read struct {
	Path   string `json:"path"`
	Offset int64  `json:"offset,omitempty"`
	Length int    `json:"length,omitempty"`
}
type Shell struct {
	Command   string `json:"command"`
	Directory string `json:"directory,omitempty"`
	TimeoutMS int    `json:"timeoutMs,omitempty"`
}
type Entry struct {
	Path      string `json:"path"`
	Directory bool   `json:"directory"`
	Size      int64  `json:"size"`
	Modified  string `json:"modified"`
}

func decode(raw json.RawMessage, target any) error {
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("參數包含多餘資料")
	}
	return nil
}

// 未登入 root 工作階段不註冊檔案或 Shell 能力。
func Register(peer *p2p.Peer, authorized func() bool) {
	if os.Geteuid() == 0 {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil || !filepath.IsAbs(home) {
		return
	}
	register := func(name string, handler func(context.Context, json.RawMessage) (any, error)) {
		_ = peer.RegisterCommandParams(name, func(ctx context.Context, raw json.RawMessage) (any, error) {
			if !authorized() || ctx.Err() != nil {
				return nil, errors.New("遠端工作階段不可用")
			}
			return handler(ctx, raw)
		})
	}
	register("files.roots", func(ctx context.Context, raw json.RawMessage) (any, error) {
		var in struct{}
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		return map[string]any{"roots": []string{home}, "readOnly": true, "maxReadBytes": 4096, "maxSearchEntries": 20}, nil
	})
	register("files.search", func(ctx context.Context, raw json.RawMessage) (any, error) {
		var in Search
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		return search(ctx, home, in)
	})
	register("files.read", func(ctx context.Context, raw json.RawMessage) (any, error) {
		var in Read
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		return read(ctx, home, in)
	})
	register("shell.run", func(ctx context.Context, raw json.RawMessage) (any, error) {
		var in Shell
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		if strings.TrimSpace(in.Command) == "" || len(in.Command) > 4096 || strings.ContainsRune(in.Command, 0) {
			return nil, errors.New("Shell 指令無效或過長")
		}
		if in.TimeoutMS == 0 {
			in.TimeoutMS = 3000
		}
		if in.TimeoutMS < 100 || in.TimeoutMS > 3000 {
			return nil, errors.New("執行期限須為 100～3000 毫秒")
		}
		if in.Directory == "" {
			in.Directory = home
		}
		if !filepath.IsAbs(in.Directory) || strings.ContainsRune(in.Directory, 0) {
			return nil, errors.New("工作目錄須為遠端絕對路徑")
		}
		run, cancel := context.WithTimeout(ctx, time.Duration(in.TimeoutMS)*time.Millisecond)
		defer cancel()
		return runShell(run, in)
	})
}
func relative(home, path string) (string, error) {
	if path == "" {
		return ".", nil
	}
	if strings.ContainsRune(path, 0) || len(path) > 2048 {
		return "", errors.New("路徑無效或過長")
	}
	if filepath.IsAbs(path) {
		r, err := filepath.Rel(home, path)
		if err != nil {
			return "", err
		}
		path = r
	}
	path = filepath.Clean(path)
	if !filepath.IsLocal(path) {
		return "", errors.New("路徑必須位於遠端使用者家目錄內")
	}
	return path, nil
}
func read(ctx context.Context, home string, in Read) (any, error) {
	if in.Offset < 0 || in.Offset > 1<<53 {
		return nil, errors.New("讀取位移無效")
	}
	if in.Length == 0 {
		in.Length = 4096
	}
	if in.Length < 1 || in.Length > 4096 {
		return nil, errors.New("每次最多讀取 4096 bytes")
	}
	name, err := relative(home, in.Path)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(home)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("僅能讀取一般檔案，不接受連結或特殊裝置")
	}
	f, err := openRegular(root, name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	info, err = f.Stat()
	if err != nil {
		return nil, err
	}
	data := make([]byte, in.Length)
	n, err := f.ReadAt(data, in.Offset)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	data = data[:n]
	// 固定 Base64，避免 UTF-8 分塊邊界或 JSON 跳脫膨脹造成內容遺失。
	return map[string]any{"path": filepath.Join(home, name), "offset": in.Offset, "nextOffset": in.Offset + int64(n), "bytes": n, "size": info.Size(), "modified": info.ModTime().UTC().Format(time.RFC3339Nano), "eof": in.Offset+int64(n) >= info.Size(), "encoding": "base64", "data": base64.StdEncoding.EncodeToString(data), "utf8": utf8.Valid(data)}, nil
}
func search(ctx context.Context, home string, in Search) (any, error) {
	if len(in.Query) > 256 || len(in.Content) > 256 {
		return nil, errors.New("搜尋字串過長")
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Depth == 0 {
		in.Depth = 4
	}
	if in.Limit < 1 || in.Limit > 20 || in.Depth < 1 || in.Depth > 8 {
		return nil, errors.New("結果上限 1～20，搜尋深度 1～8")
	}
	name, err := relative(home, in.Path)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(home)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	ctx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer cancel()
	entries := []Entry{}
	visited, skipped, budget := 0, 0, 0
	truncated := false
	stopped := false
	query := strings.ToLower(in.Query)
	var walk func(string, int) error
	walk = func(dir string, depth int) error {
		if stopped {
			return nil
		}
		if err := ctx.Err(); err != nil {
			truncated = true
			return err
		}
		info, err := root.Lstat(dir)
		if err != nil || !info.IsDir() {
			if err == nil {
				err = errors.New("搜尋路徑不是一般目錄")
			}
			return err
		}
		f, err := openDirectory(root, dir)
		if err != nil {
			return err
		}
		defer f.Close()
		for {
			items, readErr := f.ReadDir(64)
			for _, item := range items {
				if stopped || ctx.Err() != nil || visited >= 5000 || len(entries) >= in.Limit {
					stopped = true
					truncated = true
					return nil
				}
				visited++
				path := filepath.Join(dir, item.Name())
				if item.Type()&os.ModeSymlink != 0 {
					skipped++
					continue
				}
				info, e := item.Info()
				if e != nil {
					skipped++
					continue
				}
				match := strings.Contains(strings.ToLower(item.Name()), query)
				if match && in.Content != "" {
					match = false
					if info.Mode().IsRegular() {
						file, e := openRegular(root, path)
						if e == nil {
							data, e := io.ReadAll(io.LimitReader(file, 65536))
							file.Close()
							if e == nil && utf8.Valid(data) && !bytes.ContainsRune(data, 0) {
								match = bytes.Contains(data, []byte(in.Content))
							}
							if info.Size() > 65536 {
								truncated = true
							}
						} else {
							skipped++
						}
					}
				}
				if match {
					entry := Entry{filepath.Join(home, path), info.IsDir(), info.Size(), info.ModTime().UTC().Format(time.RFC3339Nano)}
					b, _ := json.Marshal(entry)
					if budget+len(b) > 11000 {
						stopped = true
						truncated = true
						return nil
					}
					budget += len(b) + 1
					entries = append(entries, entry)
				}
				if info.IsDir() {
					if depth < in.Depth {
						if e = walk(path, depth+1); e != nil {
							skipped++
						}
					} else {
						truncated = true
					}
				}
				if stopped || ctx.Err() != nil || visited >= 5000 || len(entries) >= in.Limit {
					stopped = true
					truncated = true
					return nil
				}
			}
			if readErr == io.EOF {
				return nil
			}
			if readErr != nil {
				return readErr
			}
		}
	}
	err = walk(name, 1)
	if err != nil && len(entries) == 0 {
		return nil, err
	}
	return map[string]any{"entries": entries, "visited": visited, "skipped": skipped, "truncated": truncated, "contentBytesPerFile": 65536}, nil
}
