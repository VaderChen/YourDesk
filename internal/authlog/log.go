// Package authlog 提供有界、背景寫入的診斷日誌；不接收密碼、金鑰或 SDP。
package authlog

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Enabled 僅保留舊建置參數；真正開關仍由「封包分析」控制。
var Enabled string

const (
	maxLogBytes    = 5 << 20
	maxEventBytes  = 64 << 10
	maxQueuedBytes = 4 << 20
	gateInterval   = 250 * time.Millisecond
)

type gateState struct{ enabled bool }
type metadata struct{ version, mode string }
type logRecord struct {
	state   *gateState
	data    []byte
	stack   bool
	startup bool
	barrier chan struct{}
}
type logSink interface {
	io.WriteCloser
	Sync() error
}
type logger struct {
	dir           string
	meta          atomic.Pointer[metadata]
	gate          atomic.Pointer[gateState]
	gateMu        sync.Mutex // 僅開關 API 與背景輪詢使用，Event 不取得此鎖。
	queue         chan logRecord
	queuedBytes   atomic.Int64
	dropped       atomic.Uint64
	stackRequests atomic.Uint32
	stop          chan struct{}
	done          chan struct{}
	stopOnce      sync.Once
	// 僅寫入工作者存取，測試可在啟動前注入慢速 sink。
	sink    logSink
	written int
	dirty   bool
}

var current atomic.Pointer[logger]
var currentMu sync.Mutex

func logDir() string {
	if d := os.Getenv("YOURDESK_AUTH_LOG_DIR"); d != "" {
		return d
	}
	cache, _ := os.UserCacheDir()
	return filepath.Join(cache, "YourDesk", "Logs")
}
func newLogger(dir string, sink logSink) *logger {
	l := &logger{dir: dir, queue: make(chan logRecord, 128), stop: make(chan struct{}), done: make(chan struct{}), sink: sink}
	l.gate.Store(&gateState{})
	return l
}
func activeLogger() *logger {
	dir := logDir() // 僅讀取環境設定，不存取檔案系統。
	if l := current.Load(); l != nil && l.dir == dir {
		return l
	}
	currentMu.Lock()
	defer currentMu.Unlock()
	if l := current.Load(); l != nil && l.dir == dir {
		return l
	}
	l := newLogger(dir, nil)
	old := current.Swap(l)
	if old != nil {
		old.stopOnce.Do(func() { close(old.stop) })
	}
	go l.run()
	go l.watchGate()
	return l
}
func (l *logger) setGate(enabled bool) {
	for {
		old := l.gate.Load()
		if old.enabled == enabled {
			return
		}
		// 每次切換使用新世代，避免關閉前排隊的紀錄在重新開啟後補寫。
		if l.gate.CompareAndSwap(old, &gateState{enabled: enabled}) {
			return
		}
	}
}
func (l *logger) refreshGate() {
	l.gateMu.Lock()
	defer l.gateMu.Unlock()
	_, err := os.Stat(filepath.Join(l.dir, "network-debug.enabled"))
	l.setGate(err == nil)
}
func (l *logger) watchGate() {
	l.refreshGate()
	ticker := time.NewTicker(gateInterval)
	defer ticker.Stop()
	for {
		select {
		case <-l.stop:
			return
		case <-ticker.C:
			l.refreshGate()
		}
	}
}

// IsEnabled 僅讀取快取；跨程序開關由背景輪詢同步，不在網路回呼內 Stat。
func IsEnabled() bool { return activeLogger().gate.Load().enabled }
func SetDebug(enabled bool) error {
	l := activeLogger()
	l.gateMu.Lock()
	defer l.gateMu.Unlock()
	path := filepath.Join(l.dir, "network-debug.enabled")
	if enabled {
		if err := os.MkdirAll(l.dir, 0700); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte("1"), 0600); err != nil {
			return err
		}
	} else if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	l.setGate(enabled)
	return nil
}
func Start(version, mode string) {
	l := activeLogger()
	l.meta.Store(&metadata{version: version, mode: mode})
	l.refreshGate() // 啟動時讀取一次；之後的事件不做檔案 I/O。
	l.event("startup", nil)
}
func Event(event string, fields map[string]any) { activeLogger().event(event, fields) }
func encodeRecord(event string, fields map[string]any) []byte {
	b, err := json.Marshal(map[string]any{"time": time.Now().UTC().Format(time.RFC3339Nano), "event": event, "fields": fields})
	if err != nil {
		return nil
	}
	return append(b, '\n')
}
func (l *logger) event(event string, fields map[string]any) {
	state := l.gate.Load()
	if !state.enabled {
		return
	}
	// 先序列化成獨立資料，呼叫者返回後修改 map 不會與背景工作者競爭。
	data := encodeRecord(event, fields)
	if len(data) == 0 {
		return
	}
	if len(data) > maxEventBytes {
		l.dropped.Add(1)
		return
	}
	l.enqueue(logRecord{state: state, data: data, startup: event == "startup"})
}
func (l *logger) enqueue(r logRecord) bool {
	if l.gate.Load() != r.state || !r.state.enabled {
		return false
	}
	select {
	case <-l.stop:
		return false
	default:
	}
	size := int64(len(r.data))
	for {
		old := l.queuedBytes.Load()
		if old+size > maxQueuedBytes {
			l.dropped.Add(1)
			return false
		}
		if l.queuedBytes.CompareAndSwap(old, old+size) {
			break
		}
	}
	select {
	case l.queue <- r:
		return true
	default:
		l.queuedBytes.Add(-size)
		l.dropped.Add(1)
		return false
	}
}

// Stacks 只排入工作；收集堆疊與 JSON 編碼均由背景執行，每程序最多兩次。
func Stacks() { activeLogger().stacks() }
func (l *logger) stacks() {
	state := l.gate.Load()
	if !state.enabled {
		return
	}
	for {
		n := l.stackRequests.Load()
		if n >= 2 {
			return
		}
		if l.stackRequests.CompareAndSwap(n, n+1) {
			break
		}
	}
	if !l.enqueue(logRecord{state: state, stack: true}) {
		l.stackRequests.Add(^uint32(0))
	}
}
func (l *logger) open() bool {
	if l.sink != nil {
		return true
	}
	meta := l.meta.Load()
	if meta == nil || meta.mode == "" {
		return false
	}
	if err := os.MkdirAll(l.dir, 0700); err != nil {
		return false
	}
	entries, _ := os.ReadDir(l.dir)
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".jsonl") {
			if info, err := entry.Info(); err == nil && time.Since(info.ModTime()) > 7*24*time.Hour {
				_ = os.Remove(filepath.Join(l.dir, entry.Name()))
			}
		}
	}
	pid, _ := json.Marshal(os.Getpid())
	name := time.Now().Format("20060102-150405") + "-" + meta.mode + "-" + string(pid) + ".jsonl"
	f, err := os.OpenFile(filepath.Join(l.dir, name), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return false
	}
	l.sink = f
	if info, err := f.Stat(); err == nil {
		l.written = int(info.Size())
	}
	return true
}
func (l *logger) write(data []byte) {
	if l.sink == nil || len(data) == 0 || l.written+len(data) > maxLogBytes {
		return
	}
	n, _ := l.sink.Write(data)
	l.written += n
	l.dirty = l.dirty || n > 0
}
func (l *logger) sync() {
	if l.sink != nil && l.dirty {
		_ = l.sink.Sync()
		l.dirty = false
	}
}
func (l *logger) run() {
	defer close(l.done)
	defer func() {
		l.sync()
		if l.sink != nil {
			_ = l.sink.Close()
		}
	}()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-l.stop:
			return
		case <-ticker.C:
			l.sync()
		case r := <-l.queue:
			l.queuedBytes.Add(-int64(len(r.data)))
			if r.barrier != nil {
				l.sync()
				close(r.barrier)
				continue
			}
			if l.gate.Load() != r.state || !r.state.enabled {
				continue
			}
			if !l.open() || l.gate.Load() != r.state {
				continue
			}
			if meta := l.meta.Load(); l.written == 0 && meta != nil {
				l.write(encodeRecord("startup", map[string]any{"version": meta.version, "mode": meta.mode, "pid": os.Getpid(), "parentPid": os.Getppid()}))
			}
			if r.stack {
				buf := make([]byte, 512<<10)
				n := runtime.Stack(buf, true)
				r.data = encodeRecord("goroutines", map[string]any{"stack": string(buf[:n]), "truncated": n == len(buf)})
			}
			if l.gate.Load() != r.state {
				continue
			}
			if skipped := l.dropped.Swap(0); skipped > 0 {
				l.write(encodeRecord("diagnostic-events-dropped", map[string]any{"count": skipped}))
			}
			// 檔案建立時已寫過 startup，避免重複標頭。
			if !r.startup {
				l.write(r.data)
			}
		}
	}
}

// flush 僅供停機／測試等待；一般收送路徑不得呼叫。
func (l *logger) flush(ctx context.Context) error {
	barrier := make(chan struct{})
	select {
	case l.queue <- logRecord{barrier: barrier}:
	case <-ctx.Done():
		return ctx.Err()
	case <-l.done:
		return nil
	}
	select {
	case <-barrier:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-l.done:
		return nil
	}
}

// ErrorClass 僅分類錯誤，不把服務端原始內容或連線資料寫入 LOG。
func ErrorClass(err error) string {
	if err == nil {
		return "none"
	}
	v := strings.ToLower(err.Error())
	for _, marker := range []string{"replaced", "duplicate", "already", "timeout", "deadline", "closed", "eof", "room", "role", "binding", "hmac"} {
		if strings.Contains(v, marker) {
			return marker
		}
	}
	return "other"
}
