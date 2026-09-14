package authlog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func startTestLogger(t *testing.T, l *logger) {
	t.Helper()
	go l.run()
	t.Cleanup(func() {
		l.stopOnce.Do(func() { close(l.stop) })
		select {
		case <-l.done:
		case <-time.After(time.Second):
			t.Error("診斷工作者未停止")
		}
	})
}
func flushTestLogger(t *testing.T, l *logger) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := l.flush(ctx); err != nil {
		t.Fatal(err)
	}
}
func globalTestLogger(t *testing.T) *logger {
	t.Helper()
	t.Setenv("YOURDESK_AUTH_LOG_DIR", t.TempDir())
	l := activeLogger()
	t.Cleanup(func() {
		l.stopOnce.Do(func() { close(l.stop) })
		select {
		case <-l.done:
		case <-time.After(time.Second):
			t.Error("診斷工作者未停止")
		}
		current.CompareAndSwap(l, nil)
	})
	return l
}
func TestDiagnosticBounds(t *testing.T) {
	l := globalTestLogger(t)
	Enabled = "1" // 舊旗標不啟用紀錄。
	Start("test", "viewer")
	flushTestLogger(t, l)
	files, _ := filepath.Glob(filepath.Join(l.dir, "*.jsonl"))
	if len(files) != 0 {
		t.Fatal("停用時建立紀錄")
	}
	if err := SetDebug(true); err != nil {
		t.Fatal(err)
	}
	Stacks()
	Stacks()
	Stacks()
	flushTestLogger(t, l)
	files, _ = filepath.Glob(filepath.Join(l.dir, "*.jsonl"))
	if len(files) != 1 {
		t.Fatal(files)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	startup, stacks := 0, 0
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte{'\n'}) {
		var r struct {
			Event string `json:"event"`
		}
		if err := json.Unmarshal(line, &r); err != nil {
			t.Fatal(err)
		}
		if r.Event == "startup" {
			startup++
		}
		if r.Event == "goroutines" {
			stacks++
		}
	}
	if startup != 1 || stacks != 2 {
		t.Fatalf("startup=%d stacks=%d", startup, stacks)
	}
	info, err := os.Stat(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 || info.Size() > maxLogBytes {
		t.Fatal(info)
	}
	if err := SetDebug(false); err != nil {
		t.Fatal(err)
	}
	Event("disabled", nil)
	Stacks()
	flushTestLogger(t, l)
	after, _ := os.ReadFile(files[0])
	if !bytes.Equal(data, after) {
		t.Fatal("關閉後仍新增日誌")
	}
}

type testSink struct {
	bytes.Buffer
	syncs int
}

func (s *testSink) Sync() error  { s.syncs++; return nil }
func (s *testSink) Close() error { return nil }

type blockedSink struct {
	testSink
	entered, release chan struct{}
	once             sync.Once
	blockSync        bool
}

func (s *blockedSink) block() { s.once.Do(func() { close(s.entered); <-s.release }) }
func (s *blockedSink) Write(b []byte) (int, error) {
	if !s.blockSync {
		s.block()
	}
	return s.Buffer.Write(b)
}
func (s *blockedSink) Sync() error {
	if s.blockSync {
		s.block()
	}
	return s.testSink.Sync()
}

func TestSlowLogWriterDoesNotBlockProducers(t *testing.T) {
	sink := &blockedSink{entered: make(chan struct{}), release: make(chan struct{})}
	l := newLogger(t.TempDir(), sink)
	l.setGate(true)
	startTestLogger(t, l)
	// 先解除慢速 I/O，才讓 Cleanup 等工作者結束。
	defer close(sink.release)
	l.event("first", nil)
	select {
	case <-sink.entered:
	case <-time.After(time.Second):
		t.Fatal("未進入慢速寫入")
	}
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			l.event("sample", map[string]any{"n": i})
		}
		l.stacks()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("診斷事件被磁碟寫入阻塞")
	}
	if l.dropped.Load() == 0 {
		t.Fatal("滿載時未統計略過事件")
	}
	if len(l.queue) > 128 || l.queuedBytes.Load() > maxQueuedBytes {
		t.Fatal("超過佇列上限")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := l.flush(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("flush 無法取消：%v", err)
	}
}

func TestDisableDiscardsQueuedGeneration(t *testing.T) {
	sink := &blockedSink{entered: make(chan struct{}), release: make(chan struct{}), blockSync: true}
	l := newLogger(t.TempDir(), sink)
	l.setGate(true)
	startTestLogger(t, l)
	var released bool
	defer func() {
		if !released {
			close(sink.release)
		}
	}()
	l.event("written", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	flushed := make(chan error, 1)
	go func() { flushed <- l.flush(ctx) }()
	select {
	case <-sink.entered:
	case <-ctx.Done():
		t.Fatal("未進入慢速 Sync")
	}
	fields := map[string]any{"value": "before"}
	l.event("stale", fields)
	fields["value"] = "after"
	l.setGate(false)
	l.event("disabled", nil)
	l.setGate(true)
	l.event("fresh", nil)
	close(sink.release)
	released = true
	if err := <-flushed; err != nil {
		t.Fatal(err)
	}
	flushTestLogger(t, l)
	if strings.Contains(sink.String(), "stale") || strings.Contains(sink.String(), "disabled") || !strings.Contains(sink.String(), "fresh") {
		t.Fatal(sink.String())
	}
}

func TestSnapshotAndByteLimits(t *testing.T) {
	sink := &testSink{}
	l := newLogger(t.TempDir(), sink)
	l.setGate(true)
	fields := map[string]any{"value": "before"}
	l.event("snapshot", fields)
	fields["value"] = "after"
	for i := 0; i < 200; i++ {
		l.event("sample", map[string]any{"text": strings.Repeat("a", maxEventBytes-200)})
	}
	if l.queuedBytes.Load() > maxQueuedBytes || l.dropped.Load() == 0 {
		t.Fatal("沒有限制待寫入位元組")
	}
	startTestLogger(t, l)
	flushTestLogger(t, l)
	if !strings.Contains(sink.String(), "before") || strings.Contains(sink.String(), "after") {
		t.Fatal("非獨立事件快照")
	}
	if !strings.Contains(sink.String(), "diagnostic-events-dropped") {
		t.Fatal("沒有記錄取樣缺口")
	}
	if sink.Len() > maxLogBytes {
		t.Fatal("檔案超限")
	}
	// 接近檔案上限時，拒絕整筆事件，不寫入半份 JSON。
	near := &testSink{}
	limited := newLogger(t.TempDir(), near)
	limited.written = maxLogBytes - 1
	limited.setGate(true)
	startTestLogger(t, limited)
	limited.event("too-large", nil)
	flushTestLogger(t, limited)
	if near.Len() != 0 {
		t.Fatal("超過檔案上限仍寫入")
	}
}

func TestCrossProcessGateRefresh(t *testing.T) {
	l := globalTestLogger(t)
	Start("test", "host")
	if err := os.MkdirAll(l.dir, 0700); err != nil {
		t.Fatal(err)
	}
	flag := filepath.Join(l.dir, "network-debug.enabled")
	if err := os.WriteFile(flag, []byte("1"), 0600); err != nil {
		t.Fatal(err)
	}
	wait := func(want bool) {
		t.Helper()
		deadline := time.Now().Add(time.Second)
		for IsEnabled() != want {
			if time.Now().After(deadline) {
				t.Fatal("跨程序開關未同步")
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	wait(true)
	if err := os.Remove(flag); err != nil {
		t.Fatal(err)
	}
	wait(false)
}
