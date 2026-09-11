package streampipeline

import (
	"context"
	"testing"
	"time"
)

func TestConsumerOrder(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got := make(chan int, 1000)
	c := NewConsumer(ctx, func(v int) { got <- v })
	defer c.Close()
	for i := 0; i < 1000; i++ {
		if !c.Submit(i) {
			t.Fatal("unexpected cancellation")
		}
	}
	for i := 0; i < 1000; i++ {
		select {
		case v := <-got:
			if v != i {
				t.Fatalf("got %d want %d", v, i)
			}
		case <-ctx.Done():
			t.Fatal("decode timeout")
		}
	}
}

func TestConsumerBackpressureAndClose(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	entered, release := make(chan struct{}), make(chan struct{})
	c := NewConsumer(ctx, func(v int) {
		if v != 1 {
			t.Errorf("queued frame decoded after cancellation: %d", v)
		}
		close(entered)
		<-release
	})
	defer c.Close()
	// Always unblock native work before deferred Close, even on a failing assertion.
	defer close(release)
	if !c.Submit(1) {
		t.Fatal("first submission")
	}
	<-entered
	if !c.Submit(2) {
		t.Fatal("second submission")
	}
	submitted := make(chan bool, 1)
	go func() { submitted <- c.Submit(3) }()
	select {
	case <-submitted:
		t.Fatal("more than two slots accepted")
	case <-time.After(20 * time.Millisecond):
	}
	closed := make(chan struct{})
	go func() { c.Close(); close(closed) }()
	select {
	case ok := <-submitted:
		if ok {
			t.Fatal("blocked submission accepted after close")
		}
	case <-ctx.Done():
		t.Fatal("submission did not unblock")
	}
	select {
	case <-closed:
		t.Fatal("close returned while decoding")
	default:
	}
	if c.Submit(4) {
		t.Fatal("accepted after close")
	}
}
