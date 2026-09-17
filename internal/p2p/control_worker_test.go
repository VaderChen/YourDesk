package p2p

import (
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestControlWorkerStallWithoutQueueOverflow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		done, release := make(chan struct{}), make(chan struct{})
		inbox := make(chan Control, 1)
		var failures atomic.Int32
		go runControlWorker(done, inbox, func(Control) { <-release }, func() { failures.Add(1); close(done) })
		inbox <- Control{Type: "key"}
		synctest.Wait()
		time.Sleep(29 * time.Second)
		synctest.Wait()
		if failures.Load() != 0 {
			t.Fatal("premature timeout")
		}
		time.Sleep(2 * time.Second)
		synctest.Wait()
		if failures.Load() != 1 {
			t.Fatal("blocked worker not detected")
		}
		close(release)
		synctest.Wait()
	})
}

func TestControlWorkerIdleAndRecovery(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		done := make(chan struct{})
		defer close(done)
		inbox := make(chan Control, 1)
		var completed atomic.Int32
		go runControlWorker(done, inbox, func(Control) { time.Sleep(5 * time.Second); completed.Add(1) }, func() { t.Error("healthy worker declared stalled") })
		for range 3 {
			inbox <- Control{Type: "key"}
			time.Sleep(10 * time.Second)
		}
		time.Sleep(time.Hour)
		synctest.Wait()
		if completed.Load() != 3 {
			t.Fatal("events lost")
		}
	})
}

func TestControlWorkerCancellationDuringHandler(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		done, release := make(chan struct{}), make(chan struct{})
		inbox := make(chan Control, 1)
		go runControlWorker(done, inbox, func(Control) { <-release }, func() { t.Error("timeout after cancellation") })
		inbox <- Control{Type: "key"}
		synctest.Wait()
		close(done)
		time.Sleep(time.Minute)
		close(release)
		synctest.Wait()
	})
}
