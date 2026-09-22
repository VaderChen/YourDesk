package filetransfer

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path"
	"strings"
	"sync"
	"time"
)

const maxReceipts = 64

// A hub is process-local only. Tokens/offsets are never saved to disk. Production
// peers share one bounded hub; tests inject isolated hubs and temporary homes.
type transferHub struct {
	mu          sync.Mutex
	uploads     map[string]*upload
	receipts    map[string]*receipt
	reserved    int64
	now         func() time.Time
	maintenance sync.Once
}

type transferIdentity struct {
	filesystem bool
	owner      *session
	home       os.FileInfo
	path       string
	size       int64
	tokenHash  [sha256.Size]byte
}

type upload struct {
	transferIdentity
	parent     *os.Root
	file       *os.File
	temp, name string
	offset     int64
	touched    time.Time
}

type receipt struct {
	transferIdentity
	info      os.FileInfo
	completed time.Time
}

var processHub = newTransferHub()

func newTransferHub() *transferHub {
	return &transferHub{uploads: make(map[string]*upload), receipts: make(map[string]*receipt), now: time.Now}
}

func (h *transferHub) startMaintenance() {
	h.maintenance.Do(func() {
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				h.mu.Lock()
				h.expireLocked()
				h.mu.Unlock()
			}
		}()
	})
}

func (s *session) begin(raw json.RawMessage) (any, error) {
	var in struct {
		Path  string `json:"path"`
		Size  int64  `json:"size"`
		Token string `json:"token,omitempty"`
	}
	if err := decode(raw, &in); err != nil {
		return nil, err
	}
	name, err := relative(in.Path)
	if err != nil {
		return nil, err
	}
	if in.Token != "" && !validResumeToken(in.Token) {
		return nil, errors.New("續傳憑證必須是 64 位小寫十六進位字串")
	}
	token := in.Token
	if token == "" {
		var entropy [32]byte
		if _, err = rand.Read(entropy[:]); err != nil {
			return nil, errors.New("無法建立傳輸識別碼")
		}
		token = hex.EncodeToString(entropy[:])
	}
	id := token[:32]
	if s.uploads[id] != nil || s.receipts[id] != nil {
		resumeRaw, _ := json.Marshal(map[string]any{"id": id, "token": token, "path": name, "size": in.Size})
		out, err := s.resume(resumeRaw)
		if err != nil {
			return nil, err
		}
		result := out.(map[string]any)
		result["id"], result["resumeToken"] = id, token
		return result, nil
	}
	if in.Size < 0 || in.Size > MaxFileSize || in.Size > MaxFileSize-s.reserved || len(s.uploads) >= maxUploads {
		return nil, errors.New("此主機最多保留 4 個上傳，總大小上限 2 GiB；請取消或完成既有傳輸")
	}
	r, base, err := s.parent(name)
	if err != nil {
		return nil, err
	}
	keep := false
	defer func() {
		if !keep {
			r.Close()
		}
	}()
	if _, err = r.Lstat(base); !errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("目的檔案已存在或無法存取；不會覆寫")
	}
	temp := tempPrefix + id + ".part"
	f, err := r.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	s.uploads[id] = &upload{transferIdentity: transferIdentity{filesystem: s.filesystem != nil, owner: s, home: s.homeInfo, path: name, size: in.Size, tokenHash: sha256.Sum256([]byte(token))}, parent: r, file: f, temp: temp, name: base, touched: s.now()}
	s.reserved += in.Size
	keep = true
	return map[string]any{"id": id, "resumeToken": token, "state": "uploading", "nextOffset": int64(0)}, nil
}

func validResumeToken(token string) bool {
	if len(token) != 64 {
		return false
	}
	for _, c := range token {
		if !(c >= '0' && c <= '9') && !(c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func ownerLive(s *session) bool { return s != nil && !s.closed && (s.alive == nil || s.alive()) }

func (s *session) resume(raw json.RawMessage) (any, error) {
	var in struct {
		ID    string `json:"id"`
		Token string `json:"token"`
		Path  string `json:"path"`
		Size  int64  `json:"size"`
	}
	if err := decode(raw, &in); err != nil {
		return nil, err
	}
	name, err := relative(in.Path)
	if err != nil {
		return nil, err
	}
	if len(in.ID) != 32 || !validResumeToken(in.Token) || in.ID != in.Token[:32] || name == "." || in.Size < 0 || in.Size > MaxFileSize {
		return nil, errors.New("續傳憑證或參數無效")
	}
	u := s.uploads[in.ID]
	c := s.receipts[in.ID]
	var identity *transferIdentity
	if u != nil {
		identity = &u.transferIdentity
	} else if c != nil {
		identity = &c.transferIdentity
	}
	hash := sha256.Sum256([]byte(in.Token))
	if identity == nil || identity.filesystem != (s.filesystem != nil) || subtle.ConstantTimeCompare(identity.tokenHash[:], hash[:]) != 1 || identity.path != name || identity.size != in.Size || !os.SameFile(identity.home, s.homeInfo) {
		return nil, errors.New("續傳憑證不符、已逾時或主機已重新啟動")
	}
	if identity.owner != s && ownerLive(identity.owner) {
		return nil, errors.New("此傳輸仍由另一個已連線工作階段使用")
	}
	parent, base, err := s.parent(name)
	if err != nil {
		return nil, err
	}
	defer parent.Close()
	if c != nil {
		info, err := parent.Lstat(base)
		if err != nil {
			return nil, err
		}
		if !sameVersion(info, c.info) {
			return nil, errors.New("已完成檔案後來已變更，無法確認續傳狀態")
		}
		c.owner = s
		return map[string]any{"state": "complete", "nextOffset": c.size}, nil
	}
	wantParent, err := u.parent.Stat(".")
	if err != nil {
		return nil, err
	}
	currentParent, err := parent.Stat(".")
	if err != nil {
		return nil, err
	}
	staged, err := u.file.Stat()
	if err != nil {
		return nil, err
	}
	current, err := u.parent.Lstat(u.temp)
	if err != nil {
		return nil, err
	}
	if !os.SameFile(wantParent, currentParent) || !current.Mode().IsRegular() || !os.SameFile(current, staged) || staged.Size() != u.offset {
		return nil, errors.New("暫存檔案或目的目錄已變更，請取消後重新上傳")
	}
	if _, err = parent.Lstat(base); !errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("目的檔案已存在或無法存取；不會覆寫")
	}
	u.owner = s
	u.touched = s.now()
	return map[string]any{"state": "uploading", "nextOffset": u.offset}, nil
}

func (s *session) write(raw json.RawMessage) (any, error) {
	var in struct {
		ID     string `json:"id"`
		Offset int64  `json:"offset"`
		Data   string `json:"data"`
	}
	if err := decode(raw, &in); err != nil {
		return nil, err
	}
	if len(in.ID) != 32 || len(in.Data) > base64.StdEncoding.EncodedLen(ChunkSize) {
		return nil, errors.New("傳輸資料無效或過大")
	}
	u := s.uploads[in.ID]
	if u == nil || u.owner != s {
		return nil, errors.New("傳輸不存在、未續接或已逾時")
	}
	data, err := base64.StdEncoding.Strict().DecodeString(in.Data)
	if err != nil || len(data) < 1 || len(data) > ChunkSize || in.Offset != u.offset || int64(len(data)) > u.size-u.offset {
		return nil, errors.New("傳輸資料或連續位移無效")
	}
	n, err := u.file.Write(data)
	if err != nil || n != len(data) {
		s.removeUpload(in.ID)
		return nil, errors.New("寫入失敗，已取消此上傳")
	}
	u.offset += int64(n)
	u.touched = s.now()
	return map[string]int64{"nextOffset": u.offset}, nil
}

func (s *session) finish(ctx context.Context, method string, raw json.RawMessage) (any, error) {
	if method == "cancel" {
		return s.cancel(raw)
	}
	var in struct {
		ID string `json:"id"`
	}
	if err := decode(raw, &in); err != nil {
		return nil, err
	}
	if len(in.ID) != 32 {
		return nil, errors.New("傳輸識別碼無效")
	}
	u, c := s.uploads[in.ID], s.receipts[in.ID]
	if (u != nil && u.owner != s) || (c != nil && c.owner != s) {
		return nil, errors.New("請先以續傳憑證續接此傳輸")
	}
	if c != nil {
		return map[string]bool{"ok": true}, nil
	}
	if u == nil {
		return nil, errors.New("傳輸不存在或已逾時")
	}
	if u.offset != u.size {
		return nil, errors.New("檔案尚未完整傳入")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := u.file.Sync(); err != nil {
		return nil, err
	}
	info, err := u.file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() != u.size {
		return nil, errors.New("暫存檔案大小已變更")
	}
	// A retained parent handle may outlive a local rename. Never silently
	// commit into its new location when the originally requested path changed.
	currentParent, _, err := s.parent(u.path)
	if err != nil {
		return nil, err
	}
	expectedParent, err := u.parent.Stat(".")
	presentParent, presentErr := currentParent.Stat(".")
	currentParent.Close()
	if err != nil || presentErr != nil || !os.SameFile(expectedParent, presentParent) {
		return nil, errors.New("目的目錄已移動或替換，請取消後重新上傳")
	}
	if err = commitNoReplace(u.parent, u.temp, u.name, info); err != nil {
		return nil, err
	}
	// Windows finalizes the last-write timestamp when write handles close.
	// Capture the committed receipt afterwards, not from the still-open stage.
	u.file.Close()
	receiptInfo := info
	if finalInfo, finalErr := u.parent.Lstat(u.name); finalErr == nil && finalInfo.Mode().IsRegular() && os.SameFile(info, finalInfo) && finalInfo.Size() == u.size {
		receiptInfo = finalInfo
	}
	for len(s.receipts) >= maxReceipts {
		var oldest string
		for id, item := range s.receipts {
			if oldest == "" || item.completed.Before(s.receipts[oldest].completed) {
				oldest = id
			}
		}
		delete(s.receipts, oldest)
	}
	s.receipts[in.ID] = &receipt{transferIdentity: u.transferIdentity, info: receiptInfo, completed: s.now()}
	s.releaseUpload(in.ID, info)
	return map[string]bool{"ok": true}, nil
}

func (h *transferHub) removeUpload(id string) {
	u := h.uploads[id]
	if u == nil {
		return
	}
	owned, _ := u.file.Stat()
	h.releaseUpload(id, owned)
}

func (h *transferHub) releaseUpload(id string, owned os.FileInfo) {
	u := h.uploads[id]
	if u == nil {
		return
	}
	// Cancellation/expiry owns the staged inode, not any file a local process
	// might later put under the same name. In particular, never unlink a
	// replacement symlink, directory, or unrelated regular file.
	current, currentErr := u.parent.Lstat(u.temp)
	u.file.Close()
	if owned != nil && currentErr == nil && owned.Mode().IsRegular() && current.Mode().IsRegular() && os.SameFile(owned, current) {
		_ = u.parent.Remove(u.temp)
	}
	u.parent.Close()
	h.reserved -= u.size
	delete(h.uploads, id)
}

func (h *transferHub) expireLocked() {
	for id, u := range h.uploads {
		if h.now().Sub(u.touched) >= uploadIdle {
			h.removeUpload(id)
		}
	}
	for id, c := range h.receipts {
		if h.now().Sub(c.completed) >= uploadIdle {
			delete(h.receipts, id)
		}
	}
}

func (s *session) cancelAllLocked() {
	for id, u := range s.uploads {
		if u.owner == s {
			s.removeUpload(id)
		}
	}
	for id, c := range s.receipts {
		if c.owner == s {
			delete(s.receipts, id)
		}
	}
}

// detach preserves only bounded in-memory metadata and private staged files.
// A new authorized peer must prove token possession before it can write/cancel.
func (s *session) detach() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	for _, u := range s.uploads {
		if u.owner == s {
			u.owner = nil
		}
	}
	for _, c := range s.receipts {
		if c.owner == s {
			c.owner = nil
		}
	}
	s.root.Close()
}

// close cancels this session's owned work (used for explicit local teardown in
// tests). Peer disconnect uses detach, not this destructive variant.
func (s *session) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	s.cancelAllLocked()
	s.root.Close()
}

func (h *transferHub) close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id := range h.uploads {
		h.removeUpload(id)
	}
	clear(h.receipts)
}

func sameVersion(a, b os.FileInfo) bool {
	return a != nil && b != nil && a.Mode().Type() == b.Mode().Type() && os.SameFile(a, b) && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}

func (s *session) conflictsWithUpload(name string, directory bool) bool {
	for _, u := range s.uploads {
		if u.filesystem != (s.filesystem != nil) || !os.SameFile(u.home, s.homeInfo) {
			continue
		}
		// Actual protected staging names are checked again during traversal; this
		// conservative lexical check also covers common case-insensitive aliases.
		for current := u.path; current != "."; current = path.Dir(current) {
			if strings.EqualFold(current, name) {
				return true
			}
			if !directory {
				break
			}
		}
	}
	return false
}
