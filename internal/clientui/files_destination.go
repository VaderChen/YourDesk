package clientui

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

type localDirectoryEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type localDirectoryList struct {
	Path       string                `json:"path"`
	Parent     string                `json:"parent"`
	Entries    []localDirectoryEntry `json:"entries"`
	NextOffset int                   `json:"nextOffset"`
}

func localDirectoryPath(path string) (string, error) {
	if path == "" {
		return fileDownloadsDirectory()
	}
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, strings.TrimLeft(path[1:], `/\`))
	}
	if len(path) > 4096 || !utf8.ValidString(path) || strings.ContainsRune(path, 0) || !filepath.IsAbs(path) {
		return "", errors.New("下載目錄無效")
	}
	return filepath.Clean(path), nil
}

// 僅由檔案視窗的本機橋接呼叫，不向遠端傳送本機目錄或路徑。
func listLocalDirectories(ctx context.Context, path string, offset int) (localDirectoryList, error) {
	out := localDirectoryList{Entries: []localDirectoryEntry{}, NextOffset: -1}
	if offset < 0 || offset > 1000000 {
		return out, errors.New("目錄分頁無效")
	}
	clean, err := localDirectoryPath(path)
	if err != nil {
		return out, err
	}
	f, err := os.Open(clean)
	if err != nil {
		return out, errors.New("無法讀取本機目錄，請確認路徑與權限")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.IsDir() {
		return out, errors.New("請選擇本機資料夾")
	}
	out.Path, out.Parent = clean, filepath.Dir(clean)
	if out.Parent == clean {
		out.Parent = ""
	}
	for skipped := 0; skipped < offset; {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		entries, err := f.ReadDir(min(256, offset-skipped))
		skipped += len(entries)
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return out, err
		}
	}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	entries, err := f.ReadDir(256)
	if err != nil && err != io.EOF {
		return out, errors.New("無法讀取本機目錄，請確認路徑與權限")
	}
	if len(entries) == 256 {
		out.NextOffset = offset + len(entries)
	}
	for _, entry := range entries {
		path := filepath.Join(clean, entry.Name())
		isDir := entry.IsDir()
		if entry.Type()&os.ModeSymlink != 0 {
			info, err := os.Stat(path)
			isDir = err == nil && info.IsDir()
		}
		if isDir {
			out.Entries = append(out.Entries, localDirectoryEntry{entry.Name(), path})
		}
	}
	sort.Slice(out.Entries, func(i, j int) bool {
		return strings.ToLower(out.Entries[i].Name) < strings.ToLower(out.Entries[j].Name)
	})
	return out, nil
}
