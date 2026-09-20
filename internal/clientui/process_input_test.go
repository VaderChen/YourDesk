package clientui

import (
	"context"
	"errors"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type stalledProcessPipe struct {
	entered chan struct{}
	closed  chan struct{}
	start   sync.Once
	stop    sync.Once
}

func newStalledProcessPipe() *stalledProcessPipe {
	return &stalledProcessPipe{entered: make(chan struct{}), closed: make(chan struct{})}
}
func (p *stalledProcessPipe) Write([]byte) (int, error) {
	p.start.Do(func() { close(p.entered) })
	<-p.closed
	return 0, io.ErrClosedPipe
}
func (p *stalledProcessPipe) Close() error { p.stop.Do(func() { close(p.closed) }); return nil }

func waitProcessInput(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("子程序 writer 未結束")
	}
}

func TestProcessInputBoundedAndCancellation(t *testing.T) {
	pipe := newStalledProcessPipe()
	input := newProcessInput(pipe)
	defer input.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- input.Send(ctx, []byte("first\n")) }()
	waitProcessInput(t, pipe.entered)
	for i := 0; i < processInputCapacity; i++ {
		if _, err := input.Write([]byte("queued\n")); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := input.Write([]byte("overflow\n")); !errors.Is(err, errProcessInputBusy) {
		t.Fatalf("未拒絕滿載佇列：%v", err)
	}
	cancel()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("已取消的寫入成功")
		}
	case <-time.After(time.Second):
		t.Fatal("取消未解除等待")
	}
	waitProcessInput(t, input.stopped)
	if len(input.queue) != 0 {
		t.Fatal("writer 結束仍保留訊息")
	}
}

func TestProcessInputDeadlineUnblocksRealPipe(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	input := newProcessInput(writer)
	defer input.Close()
	// 填滿匿名 OS pipe；它與 os/exec.StdinPipe 相同，不用測試假物件代替 Close。
	if _, err := input.Write(make([]byte, 65536)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := input.Send(ctx, make([]byte, 65536)); err == nil {
		t.Fatal("阻塞管線未逾時")
	}
	// 第一筆由 Write 入列，使用預設期限；明確關閉讓該 writer 也被回收。
	input.Close()
	waitProcessInput(t, input.stopped)
}

func TestProcessInputSkipsCancelledQueuedMessage(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	input := newProcessInput(writer)
	defer input.Close()
	first, err := input.enqueue(context.Background(), []byte("first\n"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	second, err := input.enqueue(ctx, []byte("cancelled\n"))
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	third, err := input.enqueue(context.Background(), []byte("third\n"))
	if err != nil {
		t.Fatal(err)
	}
	data := make([]byte, len("first\nthird\n"))
	if _, err := io.ReadFull(reader, data); err != nil {
		t.Fatal(err)
	}
	if string(data) != "first\nthird\n" {
		t.Fatalf("取消命令仍被送出：%q", data)
	}
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if err := <-second; !errors.Is(err, context.Canceled) {
		t.Fatalf("取消錯誤：%v", err)
	}
	if err := <-third; err != nil {
		t.Fatal(err)
	}
	input.Close()
	waitProcessInput(t, input.stopped)
}

func TestAgentPipeCancellationReleasesGlobalMutexAndPending(t *testing.T) {
	pipe := newStalledProcessPipe()
	p := &process{kind: "viewer", stage: "connected", stdin: pipe, done: make(chan struct{})}
	s := &server{children: map[string]*process{"viewer:test": p}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var callers sync.WaitGroup
	for i := 0; i < 8; i++ {
		callers.Add(1)
		go func() {
			defer callers.Done()
			if _, err := s.callRemoteAgent(ctx, mcpAction{Session: "viewer:test", Action: "key", Key: "a"}); err == nil {
				t.Error("未傳送的操作不得成功")
			}
		}()
	}
	waitProcessInput(t, pipe.entered)
	// 最壞情境：傳送中完全沒有 stdin 消費者，UI／回覆讀取仍可取得全域鎖。
	locked := make(chan struct{})
	go func() { s.mu.Lock(); s.mu.Unlock(); close(locked) }()
	waitProcessInput(t, locked)
	cancel()
	allDone := make(chan struct{})
	go func() { callers.Wait(); close(allDone) }()
	waitProcessInput(t, allDone)
	s.mu.Lock()
	input := p.stdin.(*processInput)
	remaining := len(p.agentPending)
	s.mu.Unlock()
	waitProcessInput(t, input.stopped)
	if remaining != 0 {
		t.Fatalf("取消後仍有 %d 筆 pending", remaining)
	}
}

type firstMessageProcessPipe struct {
	*stalledProcessPipe
	first chan struct{}
	calls atomic.Int32
}

func (p *firstMessageProcessPipe) Write(data []byte) (int, error) {
	if p.calls.Add(1) == 1 {
		close(p.first)
		return len(data), nil
	}
	return p.stalledProcessPipe.Write(data)
}

func TestAgentPipeClosureFailsAlreadySentRequests(t *testing.T) {
	pipe := &firstMessageProcessPipe{stalledProcessPipe: newStalledProcessPipe(), first: make(chan struct{})}
	p := &process{kind: "viewer", stage: "connected", stdin: pipe, done: make(chan struct{})}
	s := &server{children: map[string]*process{"viewer:test": p}}
	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		if _, err := s.callRemoteAgent(context.Background(), mcpAction{Session: "viewer:test", Action: "show"}); err == nil {
			t.Error("已送出但未回覆的操作不得成功")
		}
	}()
	waitProcessInput(t, pipe.first)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	secondDone := make(chan struct{})
	go func() {
		defer close(secondDone)
		_, _ = s.callRemoteAgent(ctx, mcpAction{Session: "viewer:test", Action: "show"})
	}()
	waitProcessInput(t, pipe.entered)
	cancel()
	waitProcessInput(t, secondDone)
	waitProcessInput(t, firstDone)
	s.mu.Lock()
	remaining := len(p.agentPending)
	input := p.stdin.(*processInput)
	s.mu.Unlock()
	waitProcessInput(t, input.stopped)
	if remaining != 0 {
		t.Fatalf("已送出 pending 未清除：%d", remaining)
	}
}
