package main

import (
	"context"
	"sync/atomic"
	"testing"
	"testing/synctest"
)

func TestViewerConnectionCleanupWaitsAndIsIdempotent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		var cleanups atomic.Int32
		c := &viewerConnection{cancel: cancel, done: make(chan struct{}), cleanup: func() { cleanups.Add(1) }}
		release := make(chan struct{})
		c.workers.Add(1)
		go func() { defer c.workers.Done(); <-release }()
		c.Close()
		c.Close()
		synctest.Wait()
		if ctx.Err() == nil {
			t.Fatal("session not cancelled")
		}
		select {
		case <-c.done:
			t.Fatal("freed resources while worker active")
		default:
		}
		close(release)
		synctest.Wait()
		select {
		case <-c.done:
		default:
			t.Fatal("cleanup did not finish")
		}
		if cleanups.Load() != 1 {
			t.Fatalf("cleanups=%d", cleanups.Load())
		}
	})
}
