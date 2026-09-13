package p2p

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func TestClipboardSendWaitCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := &Peer{done: make(chan struct{}), clipboardDone: make(chan struct{})}
		if err := p.acquireClipboardSend(context.Background()); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		result := make(chan error, 1)
		go func() { result <- p.acquireClipboardSend(ctx) }()
		synctest.Wait()
		select {
		case err := <-result:
			t.Fatalf("第二個工作者越過門檻：%v", err)
		default:
		}
		time.Sleep(time.Second)
		if err := <-result; !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(err)
		}
		<-p.clipboardSendGate
		if err := p.acquireClipboardSend(context.Background()); err != nil {
			t.Fatal(err)
		}
		close(p.done)
		if err := p.acquireClipboardSend(context.Background()); err == nil {
			t.Fatal("斷線後仍等待送出")
		}
	})
}
