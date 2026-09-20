package p2p

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
)

func TestControlSenderOpenCannotMissPendingEnqueue(t *testing.T) {
	var s orderedControlSender
	var wg sync.WaitGroup
	var open atomic.Bool
	checked, release := make(chan struct{}), make(chan struct{})
	var out []string
	write := func(b []byte) error { out = append(out, string(b)); return nil }
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := s.send(func() bool {
			state := open.Load()
			close(checked)
			<-release // Channel becomes open after the initial state check.
			return state
		}, write, []byte("key-down")); err != nil {
			t.Error(err)
		}
	}()
	<-checked
	if s.mu.TryLock() {
		s.mu.Unlock()
		close(release)
		wg.Wait()
		t.Fatal("state check is not serialized with enqueue")
	}
	open.Store(true)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := s.flush(open.Load, write); err != nil {
			t.Error(err)
		}
	}()
	close(release)
	wg.Wait()
	if !reflect.DeepEqual(out, []string{"key-down"}) || len(s.pending) != 0 {
		t.Fatalf("OnOpen lost the in-flight enqueue: out=%v pending=%d", out, len(s.pending))
	}
}

func TestControlSenderNewEventCannotOvertakeOpeningQueue(t *testing.T) {
	var s orderedControlSender
	var wg sync.WaitGroup
	var out []string
	entered, release := make(chan struct{}), make(chan struct{})
	write := func(b []byte) error {
		if string(b) == "key-down" {
			close(entered)
			<-release
		}
		out = append(out, string(b))
		return nil
	}
	if err := s.send(func() bool { return false }, write, []byte("key-down")); err != nil {
		t.Fatal(err)
	}
	wg.Add(1)
	go func() { defer wg.Done(); _ = s.flush(func() bool { return true }, write) }()
	<-entered
	if s.mu.TryLock() {
		s.mu.Unlock()
		close(release)
		wg.Wait()
		t.Fatal("opening queue does not own the send lock")
	}
	wg.Add(1)
	go func() { defer wg.Done(); _ = s.send(func() bool { return true }, write, []byte("key-up")) }()
	close(release)
	wg.Wait()
	if !reflect.DeepEqual(out, []string{"key-down", "key-up"}) {
		t.Fatalf("control ordering changed: %v", out)
	}
}

func TestControlSenderFullQueueAndFailedFlushPreserveAcceptedEvents(t *testing.T) {
	var s orderedControlSender
	var out []string
	write := func(b []byte) error { out = append(out, string(b)); return nil }
	for i := 0; i < 64; i++ {
		if err := s.send(func() bool { return false }, write, []byte(fmt.Sprint(i))); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.send(func() bool { return false }, write, []byte("overflow")); !errors.Is(err, errControlQueueFull) {
		t.Fatal("silently evicted an accepted control")
	}
	failure := errors.New("send failed")
	if err := s.flush(func() bool { return true }, func([]byte) error { return failure }); !errors.Is(err, failure) || len(s.pending) != 64 {
		t.Fatal("failed flush lost queued controls")
	}
	if err := s.send(func() bool { return true }, write, []byte("last")); err != nil {
		t.Fatal(err)
	}
	if len(out) != 65 || out[0] != "0" || out[63] != "63" || out[64] != "last" || len(s.pending) != 0 {
		t.Fatalf("queue contents/order changed: %v", out)
	}
}

func TestControlSenderConcurrentOpenStress(t *testing.T) {
	for attempt := 0; attempt < 100; attempt++ {
		var s orderedControlSender
		var open atomic.Bool
		var out []string
		write := func(b []byte) error { out = append(out, string(b)); return nil }
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			for i := 0; i < 64; i++ {
				if err := s.send(open.Load, write, []byte(fmt.Sprint(i))); err != nil {
					t.Error(err)
				}
			}
		}()
		go func() {
			defer wg.Done()
			open.Store(true)
			if err := s.flush(open.Load, write); err != nil {
				t.Error(err)
			}
		}()
		wg.Wait()
		if len(out) != 64 || len(s.pending) != 0 {
			t.Fatalf("attempt %d lost controls: delivered=%d pending=%d", attempt, len(out), len(s.pending))
		}
		for i, value := range out {
			if value != fmt.Sprint(i) {
				t.Fatalf("attempt %d reordered event %d: %s", attempt, i, value)
			}
		}
	}
}
