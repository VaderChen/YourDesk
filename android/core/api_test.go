package core

import (
	"context"
	"encoding/json"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"yourdeskandroid/core/internal/p2p"
)

func TestFrameQueueWorstCaseBoundedAndRecovers(t *testing.T) {
	v := NewViewer()
	for i := 1; i <= 100000; i++ {
		v.enqueueFrame(p2p.Frame{Sequence: uint64(i), Keyframe: i == 1, JPEG: []byte{1}})
		if len(v.frames) > maxQueuedFrames || v.frameBytes > maxQueuedFrameBytes {
			t.Fatalf("unbounded queue at %d", i)
		}
	}
	if len(v.frames) != 0 || !v.awaitingKeyframe || !v.ConsumeFrameRecoveryRequest() {
		t.Fatal("overflow did not invalidate dependent frames / request recovery")
	}
	if v.ConsumeFrameRecoveryRequest() {
		t.Fatal("request not consumed")
	}
	v.enqueueFrame(p2p.Frame{Sequence: 100001, Keyframe: true, JPEG: []byte{2}})
	v.enqueueFrame(p2p.Frame{Sequence: 100002, JPEG: []byte{3}})
	if len(v.frames) != 2 || v.awaitingKeyframe || v.ConsumeFrameRecoveryRequest() {
		t.Fatal("new keyframe did not restore queue")
	}
	for _, seq := range []uint64{100001, 100002} {
		var frame struct{ Sequence uint64 }
		if err := json.Unmarshal([]byte(v.ReadFrameJSON()), &frame); err != nil || frame.Sequence != seq {
			t.Fatalf("sequence=%d err=%v", frame.Sequence, err)
		}
	}
	if v.frameBytes != 0 {
		t.Fatal("dequeue retained byte budget")
	}
}

func TestFrameQueueByteBudgetAndKeyframeOverflow(t *testing.T) {
	v := NewViewer()
	data := make([]byte, 1024*1024)
	for i := 0; i < 40; i++ {
		v.enqueueFrame(p2p.Frame{Sequence: uint64(i), Keyframe: i == 0, JPEG: data})
	}
	if len(v.frames) != 0 || !v.awaitingKeyframe || !v.ConsumeFrameRecoveryRequest() {
		t.Fatal("byte budget failed")
	}
	for i := 0; i < maxQueuedFrames+1; i++ {
		v.enqueueFrame(p2p.Frame{Sequence: uint64(i + 100), Keyframe: true, JPEG: []byte{1}})
	}
	if len(v.frames) != 1 || !v.frames[0].Keyframe || v.frameBytes != 1 {
		t.Fatal("overflowing keyframe was not safely retained")
	}
	v.enqueueFrame(p2p.Frame{Keyframe: true, JPEG: make([]byte, maxQueuedFrameBytes+1)})
	if len(v.frames) != 0 || !v.awaitingKeyframe {
		t.Fatal("oversized frame retained")
	}
}

func TestFrameQueueNoDeltaWithoutBaseline(t *testing.T) {
	v := NewViewer()
	v.enqueueFrame(p2p.Frame{Sequence: 1, JPEG: []byte{1}})
	if len(v.frames) != 0 || !v.ConsumeFrameRecoveryRequest() {
		t.Fatal("initial delta accepted")
	}
	v.enqueueFrame(p2p.Frame{Sequence: 2, Keyframe: true, JPEG: []byte{2}})
	if v.popFrame() == nil {
		t.Fatal("missing keyframe")
	}
	for i := 0; i < maxQueuedFrames+1; i++ {
		v.enqueueFrame(p2p.Frame{Sequence: uint64(i + 3), JPEG: []byte{1}})
	}
	if len(v.frames) != 0 || !v.awaitingKeyframe {
		t.Fatal("lost JPEG dependency was retained")
	}
}

func TestPopReleasesReceiveLockBeforeSerialization(t *testing.T) {
	v := NewViewer()
	v.enqueueFrame(p2p.Frame{Keyframe: true, JPEG: make([]byte, 1<<20)})
	frame := v.popFrame()
	if frame == nil || !v.frameMu.TryLock() {
		t.Fatal("serialization would retain the receive lock")
	}
	v.frameMu.Unlock()
	if len(frame.JPEG) != 1<<20 || v.frameBytes != 0 {
		t.Fatal("dequeue changed payload or accounting")
	}
}

func TestConcurrentProducerSlowReaderAndClose(t *testing.T) {
	v := NewViewer()
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		data := make([]byte, 64<<10)
		for i := 0; i < 20000; i++ {
			v.enqueueFrame(p2p.Frame{Sequence: uint64(i), Keyframe: i%120 == 0, JPEG: data})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			_ = v.ReadFrameJSON()
			_ = v.ConsumeFrameRecoveryRequest()
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			v.Close()
		}
	}()
	wg.Wait()
	if len(v.frames) > maxQueuedFrames || v.frameBytes > maxQueuedFrameBytes {
		t.Fatal("concurrent queue exceeded budget")
	}
}

func TestCloseCancelsConnectBeforeFirstDial(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		c, e := listener.Accept()
		if e == nil {
			accepted <- c
		}
	}()
	v := NewViewer()
	entered, resume := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	v.SetStateHandler(func(string) {
		if calls.Add(1) == 1 {
			close(entered)
			<-resume
		}
	})
	finished := make(chan error, 1)
	go func() { finished <- v.Connect("unused", listener.Addr().String(), "test-secret") }()
	<-entered
	v.Close()
	close(resume)
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("canceled connection succeeded")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close left a pending dial")
	}
	select {
	case c := <-accepted:
		c.Close()
		t.Fatal("dial started after Close completed")
	default:
	}
}

func TestOldGenerationCannotEnqueueAfterReconnect(t *testing.T) {
	v := NewViewer()
	ctx1, cancel1, g1 := v.beginConnect()
	defer cancel1()
	_, cancel2, g2 := v.beginConnect()
	defer cancel2()
	if ctx1.Err() == nil || g1 == g2 {
		t.Fatal("new session did not cancel prior generation")
	}
	f := p2p.Frame{Keyframe: true, JPEG: []byte{1}}
	v.enqueueSessionFrame(f, g1)
	if len(v.frames) != 0 {
		t.Fatal("old session contaminated new queue")
	}
	v.enqueueSessionFrame(f, g2)
	if len(v.frames) != 1 {
		t.Fatal("current session frame lost")
	}
	v.Close()
	v.enqueueSessionFrame(f, g2)
	if len(v.frames) != 0 {
		t.Fatal("closed session retained a late frame")
	}
}

func TestRepeatedConcurrentConnectRegistrationAndClose(t *testing.T) {
	v := NewViewer()
	var wg sync.WaitGroup
	var contextsMu sync.Mutex
	var contexts []context.Context
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				ctx, cancel, _ := v.beginConnect()
				contextsMu.Lock()
				contexts = append(contexts, ctx)
				contextsMu.Unlock()
				v.Close()
				cancel()
			}
		}()
	}
	wg.Wait()
	v.Close()
	for _, ctx := range contexts {
		if ctx.Err() == nil {
			t.Fatal("leaked registered connection context")
		}
	}
}

func TestSlowConsumerPressureWithoutClosingSession(t *testing.T) {
	v := NewViewer()
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		defer close(done)
		for i := 0; i < 10000; i++ {
			// Independent payloads model the network's ownership transfer.
			v.enqueueFrame(p2p.Frame{Sequence: uint64(i), Keyframe: i%120 == 0, JPEG: make([]byte, 64<<10)})
			v.frameMu.Lock()
			bounded := len(v.frames) <= maxQueuedFrames && v.frameBytes <= maxQueuedFrameBytes
			v.frameMu.Unlock()
			if !bounded {
				t.Error("slow reader caused unbounded retention")
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
			}
			_ = v.ReadFrameJSON()
		}
	}()
	wg.Wait()
	v.enqueueFrame(p2p.Frame{Sequence: 10001, Keyframe: true, JPEG: []byte{1}})
	if v.awaitingKeyframe {
		t.Fatal("pressure prevented recovery")
	}
}

func BenchmarkReadFrameJSON1MiB(b *testing.B) {
	v := NewViewer()
	f := p2p.Frame{Keyframe: true, JPEG: make([]byte, 1<<20)}
	b.ReportAllocs()
	b.SetBytes(int64(len(f.JPEG)))
	for i := 0; i < b.N; i++ {
		v.enqueueFrame(f)
		_ = v.ReadFrameJSON()
	}
}
