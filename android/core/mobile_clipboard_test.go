package core

import (
	"context"
	"testing"
	"yourdeskandroid/core/internal/p2p"
)

func TestMobileClipboard(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sent := make(chan p2p.Control, 3)
	v := &Viewer{generation: 2, input: newInputSender(ctx, func(c p2p.Control) error { sent <- c; return nil })}
	v.receiveStreamControl(p2p.Control{Type: "clipboard-capabilities"}, 1)
	if s := v.PollClipboardJSON(); s != "" || v.clipActive {
		t.Fatal("過期公告")
	}
	v.receiveStreamControl(p2p.Control{Type: "clipboard-capabilities"}, 2)
	v.PollClipboardJSON()
	if (<-sent).Type != "clipboard-capabilities" {
		t.Fatal("未交握")
	}
	v.receiveStreamControl(p2p.Control{Type: "clipboard-text", Clipboard: []byte("注音")}, 2)
	if v.PollClipboardJSON() != `{"text":"注音"}` || v.PollClipboardJSON() != "" {
		t.Fatal("遠端文字未交付一次")
	}
	if v.SendPasteText("近端") != nil {
		t.Fatal("手機文字未排入")
	}
	v.input.enqueue(p2p.Control{Type: "raw-key"})
	if c := <-sent; c.Type != "clipboard-text" || string(c.Clipboard) != "近端" {
		t.Fatal("剪貼簿未先送出")
	}
	if (<-sent).Type != "raw-key" {
		t.Fatal("貼上鍵順序不符")
	}
	if v.SendPasteText(string(make([]byte, 32769))) == nil {
		t.Fatal("允許過長文字")
	}
}
