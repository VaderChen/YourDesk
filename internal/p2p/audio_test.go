package p2p

import (
	"context"
	"github.com/pion/webrtc/v4"
	"sync/atomic"
	"testing"
	"time"
	"yourdesk/internal/remoteaudio"
)

func TestAudioBoundedAndControlResponsive(t *testing.T) {
	var controls atomic.Int32
	a, b := clipboardTestPeers(t, func(c Control) {
		if c.Type == "move" {
			controls.Add(1)
		}
	})
	clipboardEventually(t, func() bool {
		a.mu.RLock()
		defer a.mu.RUnlock()
		b.mu.RLock()
		defer b.mu.RUnlock()
		return a.audio != nil && b.audio != nil && a.audio.ReadyState() == webrtc.DataChannelStateOpen
	})
	for i := 1; i <= 120; i++ {
		if err := a.SendControl(Control{Type: "move", X: float64(i) / 120, Y: 0.5}); err != nil {
			t.Fatal(err)
		}
		if err := a.SendAudio(remoteaudio.Packet{Generation: 1, Sequence: uint64(i), Codec: 2, Data: make([]byte, remoteaudio.FrameBytes)}.Marshal()); err != nil {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	clipboardEventually(t, func() bool { return len(b.audioInbox) > 0 })
	if controls.Load() < 5 {
		t.Fatal("連續鍵鼠操作未傳遞")
	}
	if len(b.audioInbox) > 8 {
		t.Fatal("聲音無上限累積")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := a.CallCommand(ctx, "ping"); err != nil {
		t.Fatal("慢速聲音接收阻塞控制", err)
	}
	a.filesOnly.Store(true)
	if a.SendAudio(make([]byte, 25)) == nil {
		t.Fatal("檔案工作階段不得傳音訊")
	}
}
