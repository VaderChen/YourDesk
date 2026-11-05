package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"unicode/utf8"
	"yourdeskandroid/core/internal/p2p"
)

func validMobileClip(b []byte) bool {
	return len(b) <= 32*1024 && utf8.Valid(b) && bytes.IndexByte(b, 0) < 0
}

// PollClipboardJSON 僅由前景桌面呼叫；沿用 Host 有序文字協定，不公告檔案／圖片支援。
func (v *Viewer) PollClipboardJSON() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.clipOffered || v.input == nil {
		return ""
	}
	if !v.clipActive {
		if err := v.input.enqueue(p2p.Control{Type: "clipboard-capabilities"}); err != nil {
			return ""
		}
		v.clipActive = true
	}
	if v.pendingClip == nil {
		return ""
	}
	data, _ := json.Marshal(struct {
		Text string `json:"text"`
	}{*v.pendingClip})
	v.pendingClip = nil
	return string(data)
}

func (v *Viewer) ClipboardPasteModifier() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.clipActive {
		return 0
	}
	if v.keyboardPlatform == "darwin" {
		return 8
	}
	return 2
}

// SendPasteText 與後續 V 使用同一個有序輸入佇列，Host 先寫入剪貼簿再貼上。
func (v *Viewer) SendPasteText(value string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.clipActive || v.input == nil {
		return fmt.Errorf("剪貼簿尚未連線")
	}
	b := []byte(value)
	if !validMobileClip(b) {
		return fmt.Errorf("手機僅支援 32 KiB 以內的文字剪貼簿")
	}
	return v.input.enqueue(p2p.Control{Type: "clipboard-text", Clipboard: b})
}
