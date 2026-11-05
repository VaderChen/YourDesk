package core

import (
	"testing"
	"yourdeskandroid/core/internal/p2p"
	"yourdeskandroid/core/internal/streamconfig"
)

func TestRawKeyboardCapabilityLifecycle(t *testing.T) {
	v := &Viewer{generation: 2}
	if v.SupportsRawKeyboard() {
		t.Fatal("舊 Host 不應預設支援")
	}
	c := p2p.Control{Type: "keyboard-capabilities", StreamCapabilities: streamconfig.Advertise("host")}
	v.receiveStreamControl(c, 1)
	if v.SupportsRawKeyboard() {
		t.Fatal("過期工作階段不應啟用")
	}
	v.receiveStreamControl(c, 2)
	if !v.SupportsRawKeyboard() {
		t.Fatal("收到公告應啟用")
	}
	v.resetStreamLocked()
	if v.SupportsRawKeyboard() {
		t.Fatal("重連必須重新確認")
	}
	v.receiveStreamControl(p2p.Control{Type: "keyboard-capabilities"}, 2)
	if !v.SupportsRawKeyboard() {
		t.Fatal("早期原始按鍵 Host 未附串流能力也應可用")
	}
}
