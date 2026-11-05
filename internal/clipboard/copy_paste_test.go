package clipboard

import (
	"context"
	"testing"
	"time"
	"yourdesk/internal/p2p"
)

// 模擬遠端複製後立刻貼上，本機未變且背景剪貼簿工作仍在等待。
func TestRemoteCopyThenPasteDoesNotWaitForStaleLocalData(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := New()
	s.ctx = ctx
	s.peer = &p2p.Peer{}
	s.pasteTransfer = &pasteTransferJob{done: make(chan struct{})}
	out := make(chan string, 2)
	go s.keyboardWithSender(ctx, func(c p2p.Control) error { out <- c.Key; return nil })
	if err := s.SendCopyControl(p2p.Control{Type: "key", Key: "c", Down: true}); err != nil {
		t.Fatal(err)
	}
	s.Poll(true)
	if err := s.SendControl(p2p.Control{Type: "key", Key: "v", Down: true}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"c", "v"} {
		select {
		case got := <-out:
			if got != want {
				t.Fatal(got)
			}
		case <-time.After(time.Second * 3):
			t.Fatal("被舊剪貼簿工作阻塞")
		}
	}
}
