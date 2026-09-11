package clipboard

import (
	"archive/tar"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const pullReadSize = 256 * 1024

// 項目資訊與內容分開傳送；offset 只在來源端使用，不接受遠端提供路徑。
type pullEntry struct {
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	Directory bool   `json:"directory,omitempty"`
	offset    int64
}
type pullSnapshot struct {
	id       string
	revision int64
	file     *os.File
	entries  []pullEntry
	last     time.Time
}
type pullResult struct {
	data []byte
	err  error
}
type pullOffer struct {
	last     atomic.Int64
	revision int64
	id       string
	entries  []pullEntry
	s        *Sync
}
type pullLease struct {
	offer *pullOffer
	close func()
}
type pullState struct {
	latestOffer string
	reaped      time.Time
	sync.Mutex
	snapshots map[string]*pullSnapshot
	current   string
	pending   map[string]chan pullResult
	receiving *pullOffer
	jobs      chan packet
	cleanup   []pullLease
}

func newPullState() *pullState {
	return &pullState{snapshots: map[string]*pullSnapshot{}, pending: map[string]chan pullResult{}, jobs: make(chan packet, 128)}
}
func pullID() string {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(id[:])
}
func validPullID(id string) bool { b, e := hex.DecodeString(id); return e == nil && len(b) == 16 }
func (s *Sync) closePull() {
	p := s.pull
	p.Lock()
	defer p.Unlock()
	cleanup := p.cleanup
	p.cleanup = nil
	p.Unlock()
	for _, f := range cleanup {
		f.close()
	}
	p.Lock()
	for _, v := range p.snapshots {
		v.file.Close()
		os.Remove(v.file.Name())
	}
}
func (s *Sync) offerFiles(ctx context.Context, revision int64, paths []string) error {
	p := s.pull
	p.Lock()
	snap := p.snapshots[p.current]
	if snap != nil && snap.revision == revision {
		p.Unlock()
		return nil
	}
	// 同一份剪貼簿在公布失敗後重用快照，避免每次重試都重讀並耗盡快照名額。
	snap = nil
	for _, candidate := range p.snapshots {
		if candidate.revision == revision {
			snap = candidate
			break
		}
	}
	// 舊快照仍供正在貼上的讀取使用；閒置十分鐘後回收，不因截圖取消傳輸。
	for id, v := range p.snapshots {
		if v != snap && time.Since(v.last) > 10*time.Minute {
			v.file.Close()
			os.Remove(v.file.Name())
			delete(p.snapshots, id)
		}
	}
	if snap == nil && len(p.snapshots) >= 8 {
		p.Unlock()
		return errors.New("保留中的剪貼簿快照過多，請等既有貼上完成後再複製")
	}
	p.Unlock()
	var err error
	if snap == nil {
		file, _, err := prepareTree(ctx, paths)
		if err != nil {
			return err
		}
		keep := false
		defer func() {
			if !keep {
				file.Close()
				os.Remove(file.Name())
			}
		}()
		snap = &pullSnapshot{id: pullID(), revision: revision, file: file, last: time.Now()}
		r := tar.NewReader(file)
		for {
			h, e := r.Next()
			if e == io.EOF {
				break
			}
			if e != nil {
				return e
			}
			off, e := file.Seek(0, io.SeekCurrent)
			if e != nil {
				return e
			}
			snap.entries = append(snap.entries, pullEntry{Name: h.Name, Size: h.Size, Directory: h.Typeflag == tar.TypeDir, offset: off})
		}
		if err := validatePullEntries(snap.entries); err != nil {
			return err
		}
		if len(snap.entries) == 0 {
			return errors.New("剪貼簿快照沒有項目")
		}
		s.nativeMu.Lock()
		changed := nativeRevision() != revision
		s.nativeMu.Unlock()
		if changed {
			return errors.New("快照建立期間已複製其他內容，請重新複製")
		}
		p.Lock()
		p.snapshots[snap.id] = snap
		p.Unlock()
		keep = true
	}

	ack := make(chan error, 1)
	s.mu.Lock()
	s.acks[snap.id] = ack
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.acks, snap.id); s.mu.Unlock() }()
	if err = s.sendPacket(ctx, packet{Type: "offer-start", ID: snap.id}); err != nil {
		return err
	}
	for _, entry := range snap.entries {
		if err = s.sendPacket(ctx, packet{Type: "offer-entry", ID: snap.id, Entry: &entry}); err != nil {
			return err
		}
	}
	if err = s.sendPacket(ctx, packet{Type: "offer-end", ID: snap.id}); err != nil {
		return err
	}
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	select {
	case err = <-ack:
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return errors.New("遠端公布可貼上項目逾時")
	}
	if err != nil {
		return err
	}
	p.Lock()
	p.current = snap.id
	p.Unlock()
	slog.Info("剪貼簿快照已就緒，等待真正貼上", "項目", len(snap.entries), "快照", snap.id)
	return nil
}
func (o *pullOffer) read(index int, offset int64, out []byte) (int, error) {
	o.last.Store(time.Now().UnixNano())
	if index < 0 || index >= len(o.entries) || o.entries[index].Directory || offset < 0 {
		return 0, os.ErrInvalid
	}
	if offset >= o.entries[index].Size {
		return 0, io.EOF
	}
	n := len(out)
	if n > pullReadSize {
		n = pullReadSize
	}
	if int64(n) > o.entries[index].Size-offset {
		n = int(o.entries[index].Size - offset)
	}
	if n == 0 {
		return 0, nil
	}
	s := o.s
	id := pullID()
	ch := make(chan pullResult, pullReadSize/dataChunk+1)
	s.pull.Lock()
	s.pull.pending[id] = ch
	s.pull.Unlock()
	defer func() { s.pull.Lock(); delete(s.pull.pending, id); s.pull.Unlock() }()
	ctx, cancel := context.WithTimeout(s.clipCtx, 45*time.Second)
	defer cancel()
	if err := s.sendPacket(ctx, packet{Type: "pull-read", ID: id, Offer: o.id, Index: index, Offset: offset, Length: n}); err != nil {
		return 0, err
	}
	total := 0
	for total < n {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case v := <-ch:
			if v.err != nil {
				return 0, v.err
			}
			if len(v.data) == 0 || len(v.data) > n-total {
				return 0, io.ErrUnexpectedEOF
			}
			total += copy(out[total:n], v.data)
		}
	}
	return total, nil
}
func (s *Sync) handlePullData(data []byte) bool {
	if len(data) == 0 || data[0] != 2 {
		return false
	}
	if len(data) < 17 || len(data) > 17+dataChunk {
		return true
	}
	id := hex.EncodeToString(data[1:17])
	s.pull.Lock()
	ch := s.pull.pending[id]
	s.pull.Unlock()
	if ch != nil {
		select {
		case ch <- pullResult{data: append([]byte(nil), data[17:]...)}:
		default:
		}
	}
	return true
}
func (s *Sync) handlePull(ctx context.Context, m packet) bool {
	switch m.Type {
	case "offer-start":
		if !s.remotePull.Load() || !validPullID(m.ID) {
			return true
		}
		s.nativeMu.Lock()
		revision := nativeRevision()
		s.nativeMu.Unlock()
		s.pull.receiving = &pullOffer{id: m.ID, s: s, revision: revision}
		return true
	case "offer-entry":
		o := s.pull.receiving
		if o == nil || o.id != m.ID {
			return true
		}
		if m.Entry == nil || len(o.entries) >= maxTreeEntries {
			s.pull.receiving = nil
			return true
		}
		o.entries = append(o.entries, *m.Entry)
		return true
	case "offer-end":
		o := s.pull.receiving
		s.pull.receiving = nil
		if o == nil || o.id != m.ID {
			return true
		}
		o.last.Store(time.Now().UnixNano())
		err := validatePullEntries(o.entries)
		s.pull.Lock()
		if len(s.pull.cleanup) >= 64 {
			err = errors.New("尚未釋放的貼上清單過多，請稍後重試")
		}
		s.pull.Unlock()
		if err == nil {
			s.nativeMu.Lock()
			if nativeRevision() != o.revision {
				err = errors.New("本機已複製新內容，不覆寫剪貼簿")
			} else {
				err = nativePublishOffer(o)
			}
			if err == nil {
				s.observed = nativeRevision()
				s.sent = s.observed
				s.sentValid = true
			}
			s.nativeMu.Unlock()
		}
		reply := packet{Type: "ack", ID: m.ID}
		if err != nil {
			reply.Error = err.Error()
			slog.Warn("無法公布遠端貼上項目", "error", err)
		} else {
			s.pull.Lock()
			s.pull.latestOffer = o.id
			s.pull.Unlock()
			slog.Info("遠端檔案清單已公布，尚未傳輸內容", "項目", len(o.entries))
		}
		replyCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		_ = s.sendPacket(replyCtx, reply)
		return true
	case "pull-error":
		s.pull.Lock()
		ch := s.pull.pending[m.ID]
		s.pull.Unlock()
		if ch != nil {
			select {
			case ch <- pullResult{err: errors.New(m.Error)}:
			default:
			}
		}
		return true
	case "pull-read":
		if !validPullID(m.ID) {
			return true
		}
		select {
		case s.pull.jobs <- m:
		default:
			c, cancel := context.WithTimeout(ctx, time.Second)
			_ = s.sendPacket(c, packet{Type: "pull-error", ID: m.ID, Error: "檔案讀取佇列已滿，請重試貼上"})
			cancel()
		}
		return true
	}
	return false
}
func (s *Sync) servePull(parent context.Context, m packet) {
	var data []byte
	var err error
	s.pull.Lock()
	v := s.pull.snapshots[m.Offer]
	if v == nil || m.Index < 0 || m.Index >= len(v.entries) || m.Length < 1 || m.Length > pullReadSize || m.Offset < 0 {
		err = errors.New("快照或讀取範圍無效")
	} else {
		e := v.entries[m.Index]
		if e.Directory || m.Offset > e.Size || int64(m.Length) > e.Size-m.Offset {
			err = os.ErrInvalid
		} else {
			if m.Offset == 0 {
				slog.Info("開始依貼上要求傳輸快照內容", "快照", m.Offer, "項目", m.Index)
			}
			v.last = time.Now()
			data = make([]byte, m.Length)
			_, err = v.file.ReadAt(data, e.offset+m.Offset)
		}
	}
	s.pull.Unlock()
	ctx, cancel := context.WithTimeout(parent, 45*time.Second)
	defer cancel()
	if err != nil {
		_ = s.sendPacket(ctx, packet{Type: "pull-error", ID: m.ID, Error: fmt.Sprint(err)})
		return
	}
	id, _ := hex.DecodeString(m.ID)
	for len(data) > 0 {
		n := len(data)
		if n > dataChunk {
			n = dataChunk
		}
		buf := append([]byte{2}, id...)
		buf = append(buf, data[:n]...)
		if err = s.peer.SendClipboard(ctx, buf); err != nil {
			return
		}
		data = data[n:]
	}
}

func validatePullEntries(entries []pullEntry) error {
	if len(entries) == 0 || len(entries) > maxTreeEntries {
		return errors.New("快照項目數量無效")
	}
	seen := map[string]bool{}
	dirs := map[string]bool{}
	var total int64
	roots := 0
	for _, e := range entries {
		if !safeTreePath(e.Name) || seen[strings.ToLower(e.Name)] || e.Size < 0 || e.Size > MaxFileBytes || e.Directory && e.Size != 0 {
			return errors.New("快照含不相容路徑或大小")
		}
		parent := path.Dir(e.Name)
		if parent == "." {
			roots++
		} else if !dirs[parent] {
			return errors.New("快照缺少父目錄")
		}
		seen[strings.ToLower(e.Name)] = true
		dirs[e.Name] = e.Directory
		total += e.Size
		if total > MaxFileBytes || roots > MaxFiles {
			return errors.New("快照超過容量或根項目限制")
		}
	}
	return nil
}

// 回收已被新清單替換且閒置的資源；讀取中的清單會持續刷新時間。
func (s *Sync) reapPull() {
	p := s.pull
	p.Lock()
	if time.Since(p.reaped) < time.Minute {
		p.Unlock()
		return
	}
	p.reaped = time.Now()
	for id, v := range p.snapshots {
		if id != p.current && time.Since(v.last) > 10*time.Minute {
			v.file.Close()
			os.Remove(v.file.Name())
			delete(p.snapshots, id)
		}
	}
	var expired []pullLease
	kept := p.cleanup[:0]
	for _, l := range p.cleanup {
		if l.offer.id != p.latestOffer && time.Since(time.Unix(0, l.offer.last.Load())) > 10*time.Minute {
			expired = append(expired, l)
		} else {
			kept = append(kept, l)
		}
	}
	p.cleanup = kept
	p.Unlock()
	for _, l := range expired {
		l.close()
	}
}

func (s *Sync) pullWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case m := <-s.pull.jobs:
			s.servePull(ctx, m)
		}
	}
}
