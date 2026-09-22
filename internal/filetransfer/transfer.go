// Package filetransfer provides bounded, user-authorized file transfers for an
// authenticated P2P session. It never invokes a shell or changes the clipboard.
package filetransfer

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"yourdesk/internal/p2p"
	"yourdesk/internal/useraccess"
)

const (
	ChunkSize           = 4096
	MaxFileSize   int64 = 2 << 30
	maxUploads          = 4
	maxPathBytes        = 1024
	maxNameBytes        = 255
	pageSize            = 16
	maxListOffset       = 100000
	uploadIdle          = 24 * time.Hour
	tempPrefix          = ".yourdesk-transfer-"
)

type Entry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Directory bool   `json:"directory"`
	Size      int64  `json:"size"`
	Modified  string `json:"modified"`
	Root      bool   `json:"root,omitempty"`
}

type listResult struct {
	Path       string             `json:"path"`
	Entries    []Entry            `json:"entries"`
	NextOffset int                `json:"nextOffset"`
	Location   *directoryLocation `json:"location,omitempty"`
}

type session struct {
	*transferHub
	root       *os.Root
	homeInfo   os.FileInfo
	closed     bool
	allowed    func() bool
	alive      func() bool
	filesystem *filesystemScope
}

// Available reports whether this interactive account may offer file transfers.
func Available() bool {
	if !useraccess.Allowed() {
		return false
	}
	home, err := os.UserHomeDir()
	if err != nil || !filepath.IsAbs(home) {
		return false
	}
	root, err := os.OpenRoot(home)
	if err != nil {
		return false
	}
	root.Close()
	return true
}

// Register binds transfer commands to the peer's authorization and lifetime.
// Every request rechecks the OS identity, even after initial registration.
func Register(peer *p2p.Peer, authorized func() bool) {
	if peer == nil || authorized == nil || !useraccess.Allowed() {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil || !filepath.IsAbs(home) {
		return
	}
	s, err := newSessionWithHub(home, func() bool { return useraccess.Allowed() && authorized() }, processHub)
	if err != nil {
		return
	}
	if s.filesystem, err = systemFilesystem(home); err != nil {
		s.close()
		return
	}
	s.alive = func() bool {
		select {
		case <-peer.Done():
			return false
		default:
			return true
		}
	}
	processHub.startMaintenance()
	for _, method := range []string{"location", "list", "stat", "read", "begin", "resume", "write", "commit", "cancel", "mkdir", "remove"} {
		name := method
		_ = peer.RegisterCommandParams("xfer."+name, func(ctx context.Context, raw json.RawMessage) (any, error) {
			return s.command(ctx, name, raw)
		})
	}
	go func() {
		<-peer.Done()
		s.detach()
	}()
}

func newSession(home string, allowed func() bool) (*session, error) {
	return newSessionWithHub(home, allowed, newTransferHub())
}

func newSessionWithHub(home string, allowed func() bool, hub *transferHub) (*session, error) {
	r, err := os.OpenRoot(home)
	if err != nil {
		return nil, errors.New("無法開啟使用者目錄")
	}
	info, err := r.Stat(".")
	if err != nil {
		r.Close()
		return nil, errors.New("無法確認使用者目錄")
	}
	return &session{transferHub: hub, root: r, homeInfo: info, allowed: allowed}, nil
}

func decode(raw json.RawMessage, out any) error {
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	if len(raw) > 8192 {
		return errors.New("參數過大")
	}
	if !utf8.Valid(raw) || len(bytes.TrimSpace(raw)) == 0 || bytes.TrimSpace(raw)[0] != '{' {
		return errors.New("參數必須是有效的 JSON 物件")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return errors.New("參數格式無效")
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("參數包含多餘資料")
	}
	return nil
}

func relative(name string) (string, error) {
	if name == "" || name == "." {
		return ".", nil
	}
	if len(name) > maxPathBytes || !utf8.ValidString(name) || strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\:") {
		return "", errors.New("路徑必須是有效的相對路徑")
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." || len(part) > maxNameBytes || strings.HasPrefix(strings.ToLower(part), tempPrefix) || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return "", errors.New("檔名或路徑無效")
		}
		for _, r := range part {
			if unicode.IsControl(r) || strings.ContainsRune("<>\"|?*", r) {
				return "", errors.New("檔名含不支援的字元")
			}
		}
		base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || (len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '0' && base[3] <= '9') {
			return "", errors.New("不接受系統保留檔名")
		}
	}
	return name, nil
}

// openDir pins each directory in turn and verifies that no component is a
// symlink. os.Root remains the containment boundary if a local rename races us.
func (s *session) openDir(name string) (*os.Root, error) {
	var r *os.Root
	var err error
	if s.filesystem != nil {
		r, name, err = s.filesystem.openRoot(name)
	} else {
		r, err = s.root.OpenRoot(".")
	}
	if err != nil {
		return nil, err
	}
	if name == "." {
		return r, nil
	}
	for _, component := range strings.Split(name, "/") {
		before, err := r.Lstat(component)
		if err != nil || !before.IsDir() {
			r.Close()
			return nil, errors.New("目錄不存在或是連結")
		}
		next, err := r.OpenRoot(component)
		if err != nil {
			r.Close()
			return nil, err
		}
		after, err := next.Stat(".")
		current, checkErr := r.Lstat(component)
		r.Close()
		if err != nil || checkErr != nil || !current.IsDir() || !os.SameFile(before, after) || !os.SameFile(current, after) {
			next.Close()
			return nil, errors.New("目錄已變更，請重試")
		}
		r = next
	}
	return r, nil
}

func (s *session) parent(name string) (*os.Root, string, error) {
	name, err := relative(name)
	if err != nil {
		return nil, "", err
	}
	if name == "." {
		return nil, "", errors.New("不能以使用者目錄作為檔案")
	}
	if s.filesystem != nil && !strings.Contains(name, "/") {
		return nil, "", errors.New("不能修改磁碟根目錄")
	}
	r, err := s.openDir(path.Dir(name))
	return r, path.Base(name), err
}

func (s *session) command(ctx context.Context, method string, raw json.RawMessage) (result any, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errors.New("檔案傳輸工作階段已關閉")
	}
	if s.allowed == nil || !s.allowed() {
		return nil, errors.New("目前工作階段不允許檔案傳輸")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.expireLocked()
	// Filesystem errors contain local paths. Never return those paths over P2P.
	defer func() {
		var pe *os.PathError
		var le *os.LinkError
		if errors.As(err, &pe) || errors.As(err, &le) {
			err = errors.New("檔案操作失敗：檔案不存在、已存在、無存取權限或儲存空間不足")
		}
	}()
	switch method {
	case "location":
		if err := decode(raw, &struct{}{}); err != nil {
			return nil, err
		}
		if s.filesystem == nil {
			return map[string]string{"path": ""}, nil
		}
		return map[string]string{"path": s.filesystem.initial}, nil
	case "list":
		var in struct {
			Path   string `json:"path"`
			Offset int    `json:"offset"`
		}
		if err = decode(raw, &in); err != nil {
			return nil, err
		}
		return s.list(ctx, in.Path, in.Offset)
	case "stat":
		var in struct {
			Path string `json:"path"`
		}
		if err = decode(raw, &in); err != nil {
			return nil, err
		}
		name, e := relative(in.Path)
		if e != nil {
			return nil, e
		}
		if name == "." || (s.filesystem != nil && !strings.Contains(name, "/")) {
			root, e := s.openDir(name)
			if e != nil {
				return nil, e
			}
			defer root.Close()
			info, e := root.Stat(".")
			if e != nil {
				return nil, e
			}
			return entry(".", info), nil
		}
		parent, base, e := s.parent(name)
		if e != nil {
			return nil, e
		}
		defer parent.Close()
		info, e := parent.Lstat(base)
		if e != nil {
			return nil, e
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return nil, errors.New("僅支援一般檔案與目錄")
		}
		return entry(name, info), nil
	case "read":
		return s.read(ctx, raw)
	case "begin":
		return s.begin(raw)
	case "resume":
		return s.resume(raw)
	case "write":
		return s.write(raw)
	case "commit", "cancel":
		return s.finish(ctx, method, raw)
	case "mkdir":
		var in struct {
			Path string `json:"path"`
		}
		if err = decode(raw, &in); err != nil {
			return nil, err
		}
		parent, base, e := s.parent(in.Path)
		if e != nil {
			return nil, e
		}
		defer parent.Close()
		if err = parent.Mkdir(base, 0700); err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, nil
	case "remove":
		return s.remove(ctx, raw)
	default:
		return nil, errors.New("不支援的檔案傳輸指令")
	}
}

func entry(name string, info os.FileInfo) Entry {
	return Entry{Name: path.Base(name), Path: name, Directory: info.IsDir(), Size: info.Size(), Modified: info.ModTime().UTC().Format(time.RFC3339Nano)}
}

func (s *session) list(ctx context.Context, name string, offset int) (any, error) {
	name, err := relative(name)
	if err != nil {
		return nil, err
	}
	if offset < 0 || offset > maxListOffset {
		return nil, errors.New("目錄分頁位移超出上限")
	}
	if s.filesystem != nil && name == "." {
		return s.filesystem.listRoots(ctx, offset)
	}
	var location *directoryLocation
	if s.filesystem != nil {
		location = s.filesystem.location(name)
	}
	r, err := s.openDir(name)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	entries, err := sortedVisibleEntries(ctx, r, name, maxListOffset+pageSize)
	if err != nil {
		return nil, err
	}
	out := listResult{Path: name, Entries: []Entry{}, NextOffset: -1, Location: location}
	encodedBase, _ := json.Marshal(out)
	// Count the actual JSON representation, including HTML escapes in paths.
	// The margin also covers nextOffset gaining decimal digits during this page.
	budget := len(encodedBase) + 32
	for i := offset; i < len(entries); i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		v := entries[i]
		encoded, _ := json.Marshal(v)
		if len(out.Entries) == pageSize || budget+len(encoded) > 14000 {
			out.NextOffset = i
			break
		}
		out.Entries = append(out.Entries, v)
		budget += len(encoded) + 1
	}
	return out, nil
}

func (s *session) read(ctx context.Context, raw json.RawMessage) (any, error) {
	var in struct {
		Path     string `json:"path"`
		Offset   int64  `json:"offset"`
		Length   int    `json:"length"`
		Modified string `json:"modified"`
		Size     int64  `json:"size"`
	}
	if err := decode(raw, &in); err != nil {
		return nil, err
	}
	if in.Length == 0 {
		in.Length = ChunkSize
	}
	if in.Length < 1 || in.Length > ChunkSize || in.Offset < 0 || in.Offset > in.Size || in.Size < 0 || in.Size > MaxFileSize || in.Modified == "" {
		return nil, errors.New("讀取大小、位移或版本無效")
	}
	r, base, err := s.parent(in.Path)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	before, err := r.Lstat(base)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, errors.New("僅支援一般檔案")
	}
	f, err := openRegular(r, base)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	valid := func(info os.FileInfo) bool {
		return info.Mode().IsRegular() && os.SameFile(before, info) && info.Size() == in.Size && info.ModTime().UTC().Format(time.RFC3339Nano) == in.Modified
	}
	if !valid(info) {
		return nil, errors.New("來源檔案已變更，請重新傳輸")
	}
	data := make([]byte, min(int64(in.Length), in.Size-in.Offset))
	n, err := f.ReadAt(data, in.Offset)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	after, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !valid(after) || int64(n) != int64(len(data)) {
		return nil, errors.New("來源檔案已變更，請重新傳輸")
	}
	return map[string]any{"data": base64.StdEncoding.EncodeToString(data[:n]), "bytes": n, "nextOffset": in.Offset + int64(n), "eof": in.Offset+int64(n) == in.Size}, nil
}
