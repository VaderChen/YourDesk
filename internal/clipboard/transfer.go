package clipboard

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"yourdesk/internal/p2p"
	"yourdesk/internal/rawkey"
)

const dataChunk = 16 * 1024

// JSON 僅承載小型中繼資料；資料片段使用 1 byte 類型 + 16 byte ID + 原始資料。
type fileEntry struct {
	Directory bool   `json:"directory,omitempty"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
}
type packet struct {
	Pull        bool        `json:"pull,omitempty"`
	Offer       string      `json:"offer,omitempty"`
	Entry       *pullEntry  `json:"entry,omitempty"`
	Index       int         `json:"index,omitempty"`
	Offset      int64       `json:"offset,omitempty"`
	Length      int         `json:"length,omitempty"`
	Directories bool        `json:"directories,omitempty"`
	Type        string      `json:"type"`
	Version     int         `json:"version,omitempty"`
	ID          string      `json:"id,omitempty"`
	Kind        string      `json:"kind,omitempty"`
	Size        int64       `json:"size,omitempty"`
	Files       []fileEntry `json:"files,omitempty"`
	Digest      string      `json:"digest,omitempty"`
	Error       string      `json:"error,omitempty"`
}

func isPasteKey(c p2p.Control) bool {
	if c.Type == "key" {
		return c.Key == "v"
	}
	return c.Type == "raw-key" && c.RawKey != nil && rawkey.Name(c.RawKey.Platform, c.RawKey.Code) == "v"
}
func (s *Sync) sendPacket(ctx context.Context, p packet) error {
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return s.peer.SendClipboard(ctx, append([]byte{0}, data...))
}
func safeName(name string) bool {
	if name == "" || len(name) > 240 || name == "." || name == ".." || strings.ContainsAny(name, "/\\:\x00<>\"|?*") || strings.TrimRight(name, ". ") != name {
		return false
	}
	for _, r := range name {
		if r < 32 {
			return false
		}
	}
	stem := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	if stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" || stem == "CONIN$" || stem == "CONOUT$" {
		return false
	}
	if len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '0' && stem[3] <= '9' {
		return false
	}
	return filepath.Base(name) == name
}
func (s *Sync) transfer(parent context.Context, force bool) (resultErr error) {
	select {
	case s.transferGate <- struct{}{}:
	case <-parent.Done():
		return parent.Err()
	}
	defer func() { <-s.transferGate }()
	ctx, cancel := context.WithTimeout(parent, TransferTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	s.nativeMu.Lock()
	revision := nativeRevision()
	if (!force && revision == s.observed && (!s.retryPending || time.Now().Before(s.retryAfter))) || (s.sentValid && revision == s.sent) {
		s.nativeMu.Unlock()
		return nil
	}
	// 暫時性失敗仍保留重試機會；不得把「讀過」等同「同步成功」。
	defer func() {
		s.nativeMu.Lock()
		defer s.nativeMu.Unlock()
		if s.observed == revision {
			s.retryPending = resultErr != nil
			if s.retryPending {
				s.retryAfter = time.Now().Add(2 * time.Second)
			}
		}
	}()
	// macOS 的延遲剪貼簿讀取可能等待另一程序，不得持有接收端發布需要的鎖。
	s.nativeMu.Unlock()
	value, readRevision, err := readStableContent(readContent)
	s.nativeMu.Lock()
	revision = readRevision
	if err == nil && revision != nativeRevision() {
		err = errors.New("剪貼簿讀取期間已變更")
	}
	// 等待系統讀取期間若已收到對方內容，不把剛接收的資料再傳回去。
	if err == nil && s.sentValid && revision == s.sent {
		s.nativeMu.Unlock()
		return nil
	}
	if err == nil {
		s.observed = revision
	}
	s.nativeMu.Unlock()
	if err != nil {
		return err
	}
	if value.Kind == "" {
		return nil
	}
	if value.Kind == "files" {
		for _, name := range value.Paths {
			if isPullPath(name) {
				return nil
			}
		}
		if !s.remotePull.Load() {
			return errors.New("遠端尚未支援貼上時傳輸，請更新兩端程式")
		}
		return s.offerFiles(ctx, revision, value.Paths)
	}
	sourceValue := value
	sourceFiles := make(map[string]os.FileInfo)
	var readers []io.Reader
	var id [16]byte
	if _, err = rand.Read(id[:]); err != nil {
		return err
	}
	begin := packet{Type: "begin", ID: hex.EncodeToString(id[:]), Kind: value.Kind}
	switch value.Kind {
	case "text":
		if !validText(value.Data) {
			return errors.New("剪貼簿文字超過 1 MiB 或格式無效")
		}
		begin.Size = int64(len(value.Data))
		readers = []io.Reader{bytes.NewReader(value.Data)}
	case "image":
		if err := validImage(value.Data); err != nil {
			return err
		}
		begin.Size = int64(len(value.Data))
		readers = []io.Reader{bytes.NewReader(value.Data)}
	default:
		return errors.New("不支援的剪貼簿格式")
	}
	slog.Info("剪貼簿開始傳輸", "類型", begin.Kind, "位元組", begin.Size, "根項目", len(begin.Files))
	ack := make(chan error, 1)
	s.mu.Lock()
	s.acks[begin.ID] = ack
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.acks, begin.ID); s.mu.Unlock() }()
	completed := false
	s.startProgress(begin, "send")
	defer func() { s.finishProgress(begin.ID, "send", completed) }()
	if err = s.sendPacket(ctx, begin); err != nil {
		return err
	}
	defer func() {
		if !completed {
			abortCtx, cancel := context.WithTimeout(parent, time.Second)
			defer cancel()
			_ = s.sendPacket(abortCtx, packet{Type: "cancel", ID: begin.ID})
		}
	}()
	reader := io.MultiReader(readers...)
	checksum := sha256.New()
	buffer := make([]byte, 17+dataChunk)
	buffer[0] = 1
	copy(buffer[1:17], id[:])
	var sent int64
	// 約 2 MiB/s 上限，小片段讓控制與影像優先送出。
	pace := time.NewTicker(8 * time.Millisecond)
	defer pace.Stop()
	checkAt := time.Now()
	for {
		n, readErr := reader.Read(buffer[17:])
		if n > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-pace.C:
			}
			select {
			case ackErr := <-ack:
				if ackErr != nil {
					return ackErr
				}
				return errors.New("過早的剪貼簿完成確認")
			default:
			}
			if time.Since(checkAt) > 100*time.Millisecond {
				changed, checkErr := s.transferSourceChanged(sourceValue, sourceFiles, &revision)
				if checkErr != nil {
					return checkErr
				}
				if changed {
					return errors.New("已取消舊的剪貼簿傳輸")
				}
				checkAt = time.Now()
			}
			if err = s.peer.SendClipboard(ctx, buffer[:17+n]); err != nil {
				return err
			}
			_, _ = checksum.Write(buffer[17 : 17+n])
			sent += int64(n)
			s.updateProgress(begin.ID, "send", sent, "transferring")
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	if sent != begin.Size {
		return errors.New("傳輸期間檔案大小已變更")
	}
	s.updateProgress(begin.ID, "send", sent, "confirming")
	if err = s.sendPacket(ctx, packet{Type: "end", ID: begin.ID, Digest: hex.EncodeToString(checksum.Sum(nil))}); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err = <-ack:
	}
	if err != nil {
		return err
	}
	completed = true
	slog.Info("剪貼簿傳輸完成，遠端已寫入剪貼簿", "類型", begin.Kind, "位元組", begin.Size, "項目", len(begin.Files))
	s.nativeMu.Lock()
	s.sent = revision
	s.sentValid = true
	s.nativeMu.Unlock()
	return nil
}

type incoming struct {
	archive     *os.File
	progressEnd func(bool)
	meta        packet
	hash        hash.Hash
	received    int64
	memory      bytes.Buffer
	dir         string
	files       []*os.File
	index       int
	fileBytes   int64
	revision    int64
	activity    time.Time
}

func (in *incoming) close(remove bool) {
	if in.archive != nil {
		name := in.archive.Name()
		_ = in.archive.Close()
		_ = os.Remove(name)
		in.archive = nil
	}
	if in.progressEnd != nil {
		in.progressEnd(!remove)
		in.progressEnd = nil
	}
	for _, f := range in.files {
		_ = f.Close()
	}
	if remove && in.dir != "" {
		_ = os.RemoveAll(in.dir)
	}
}
func (s *Sync) startIncoming(meta packet) (*incoming, error) {
	id, err := hex.DecodeString(meta.ID)
	if err != nil || len(id) != 16 {
		return nil, errors.New("無效傳輸 ID")
	}
	if meta.Size < 0 {
		return nil, errors.New("無效傳輸大小")
	}
	switch meta.Kind {
	case "tree":
		if meta.Size < 1024 || meta.Size > MaxFileBytes {
			return nil, errors.New("目錄傳輸大小無效")
		}
		if err := validateTreeRoots(meta.Files); err != nil {
			return nil, err
		}
	case "text":
		if meta.Size > MaxTextBytes || len(meta.Files) != 0 {
			return nil, errors.New("文字大小無效")
		}
	case "image":
		if meta.Size < 1 || meta.Size > MaxImageBytes || len(meta.Files) != 0 {
			return nil, errors.New("圖片大小無效")
		}
	case "files":
		if meta.Size > MaxFileBytes || len(meta.Files) < 1 || len(meta.Files) > MaxFiles {
			return nil, errors.New("檔案大小或數量無效")
		}
		names := map[string]bool{}
		var total int64
		for _, f := range meta.Files {
			key := strings.ToLower(f.Name)
			if !safeName(f.Name) || names[key] || f.Size < 0 || f.Size > MaxFileBytes {
				return nil, errors.New("檔案清單無效")
			}
			names[key] = true
			total += f.Size
		}
		if total != meta.Size {
			return nil, errors.New("檔案大小不符")
		}
	default:
		return nil, errors.New("剪貼簿格式不支援")
	}
	s.nativeMu.Lock()
	revision := nativeRevision()
	s.nativeMu.Unlock()
	in := &incoming{meta: meta, hash: sha256.New(), revision: revision, activity: time.Now()}
	if meta.Kind == "files" || meta.Kind == "tree" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		root := filepath.Join(home, "Downloads", "YourDesk", "Clipboard")
		if err = os.MkdirAll(root, 0700); err != nil {
			return nil, err
		}
		in.dir, err = os.MkdirTemp(root, ".receiving-")
		if err != nil {
			return nil, err
		}
		if meta.Kind == "tree" {
			in.archive, err = os.CreateTemp(root, ".archive-")
			if err != nil {
				in.close(true)
				return nil, err
			}
			return in, nil
		}
		for _, entry := range meta.Files {
			f, err := os.OpenFile(filepath.Join(in.dir, entry.Name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				in.close(true)
				return nil, err
			}
			in.files = append(in.files, f)
		}
	}
	return in, nil
}
func (in *incoming) write(data []byte) error {
	if in.received+int64(len(data)) > in.meta.Size {
		return errors.New("剪貼簿傳輸超過宣告大小")
	}
	in.received += int64(len(data))
	_, _ = in.hash.Write(data)
	in.activity = time.Now()
	if in.meta.Kind == "tree" {
		_, err := in.archive.Write(data)
		return err
	}
	if in.meta.Kind != "files" {
		_, err := in.memory.Write(data)
		return err
	}
	for len(data) > 0 {
		for in.index < len(in.files) && in.fileBytes == in.meta.Files[in.index].Size {
			in.index++
			in.fileBytes = 0
		}
		if in.index >= len(in.files) {
			return errors.New("檔案資料超量")
		}
		n := min(int64(len(data)), in.meta.Files[in.index].Size-in.fileBytes)
		if _, err := in.files[in.index].Write(data[:int(n)]); err != nil {
			return err
		}
		data = data[int(n):]
		in.fileBytes += n
	}
	return nil
}
func (s *Sync) finishIncoming(in *incoming, digest string) error {
	if in.received != in.meta.Size || hex.EncodeToString(in.hash.Sum(nil)) != digest {
		return errors.New("剪貼簿傳輸驗證失敗")
	}
	value := content{Kind: in.meta.Kind, Data: in.memory.Bytes()}
	if value.Kind == "text" && !validText(value.Data) {
		return errors.New("剪貼簿文字無效")
	}
	if value.Kind == "image" {
		if err := validImage(value.Data); err != nil {
			return err
		}
	}
	if value.Kind == "tree" {
		if _, err := in.archive.Seek(0, io.SeekStart); err != nil {
			return err
		}
		if err := extractTree(in.archive, in.dir, in.meta.Files); err != nil {
			return err
		}
		value.Kind = "files"
	}
	if value.Kind == "files" {
		for _, f := range in.files {
			if err := f.Sync(); err != nil {
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		}
		final := filepath.Join(filepath.Dir(in.dir), "transfer-"+in.meta.ID)
		// 不覆蓋已存在的工作階段資料。
		if _, err := os.Lstat(final); !os.IsNotExist(err) {
			return errors.New("接收目錄已存在")
		}
		if err := os.Rename(in.dir, final); err != nil {
			return err
		}
		in.dir = final
		for _, entry := range in.meta.Files {
			value.Paths = append(value.Paths, filepath.Join(final, entry.Name))
		}
	}
	s.nativeMu.Lock()
	defer s.nativeMu.Unlock()
	if nativeRevision() != in.revision {
		return errors.New("本機剪貼簿已變更，不覆寫新內容")
	}
	if err := writeContent(value); err != nil {
		return err
	}
	s.observed = nativeRevision()
	s.sent = s.observed
	s.sentValid = true
	slog.Info("剪貼簿接收完成，可在本機貼上", "類型", in.meta.Kind, "位元組", in.meta.Size, "項目", len(value.Paths))
	return nil
}
func (s *Sync) receive(ctx context.Context) {
	var current *incoming
	defer func() {
		if current != nil {
			current.close(true)
		}
	}()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	respond := func(id string, err error) {
		p := packet{Type: "ack", ID: id}
		if err != nil {
			p.Error = err.Error()
			slog.Warn("剪貼簿接收未完成", "傳輸", id, "error", err)
		}
		sendCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		_ = s.sendPacket(sendCtx, p)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if current != nil && time.Since(current.activity) > time.Minute {
				respond(current.meta.ID, errors.New("剪貼簿接收逾時"))
				current.close(true)
				current = nil
			}
		case data := <-s.packets:
			if s.handlePullData(data) {
				continue
			}
			if len(data) < 1 {
				continue
			}
			if data[0] == 1 {
				if len(data) < 17 || len(data) > 17+dataChunk || current == nil || hex.EncodeToString(data[1:17]) != current.meta.ID {
					continue
				}
				if err := current.write(data[17:]); err != nil {
					respond(current.meta.ID, err)
					current.close(true)
					current = nil
				} else {
					s.updateProgress(current.meta.ID, "receive", current.received, "transferring")
				}
				continue
			}
			if data[0] != 0 {
				continue
			}
			var message packet
			if json.Unmarshal(data[1:], &message) != nil {
				continue
			}
			if s.handlePull(ctx, message) {
				continue
			}
			switch message.Type {
			case "hello":
				if message.Version == 1 {
					s.remoteDirectories.Store(message.Directories)
					s.remotePull.Store(message.Pull)
					s.remote.Store(true)
				}
			case "ack":
				s.mu.Lock()
				ack := s.acks[message.ID]
				s.mu.Unlock()
				if ack != nil {
					var err error
					if message.Error != "" {
						err = fmt.Errorf("接收端：%s", message.Error)
					}
					select {
					case ack <- err:
					default:
					}
				}
			case "begin":
				if !s.remote.Load() {
					continue
				}
				if current != nil {
					respond(current.meta.ID, errors.New("已被新的剪貼簿取代"))
					current.close(true)
					current = nil
				}
				var err error
				current, err = s.startIncoming(message)
				if err == nil {
					s.startProgress(message, "receive")
					id := message.ID
					current.progressEnd = func(ok bool) { s.finishProgress(id, "receive", ok) }
				}
				if err != nil {
					respond(message.ID, err)
				}
			case "cancel":
				if current != nil && current.meta.ID == message.ID {
					current.close(true)
					current = nil
				}
			case "end":
				if current == nil || current.meta.ID != message.ID {
					continue
				}
				s.updateProgress(current.meta.ID, "receive", current.received, "verifying")
				err := s.finishIncoming(current, message.Digest)
				respond(message.ID, err)
				current.close(err != nil)
				current = nil
			}
		}
	}
}
