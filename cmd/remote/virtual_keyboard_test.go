package main

import (
	"testing"
	"yourdesk/internal/rawkey"
	"yourdesk/internal/shortcut"
)

func TestVirtualKeyboardChords(t *testing.T) {
	for _, platform := range []string{"darwin", "windows"} {
		offset := 1000
		if platform == "windows" {
			offset += 8192
		}
		for _, name := range []string{"a", "enter", "kpenter", "kp0", "delete", "f12", "left", "capslock"} {
			code, ok := rawkey.Code(platform, name)
			if !ok {
				t.Fatal(name)
			}
			r, ok := virtualKeyRequest(offset + code + 3*512 + 16384)
			if !ok || r.Platform != platform || r.Key != name || r.Modifiers != 19 {
				t.Fatalf("bad request: %+v", r)
			}
			held := map[int]bool{}
			for _, e := range r.Chord() {
				if e.Down {
					held[e.Code] = true
				} else {
					delete(held, e.Code)
				}
			}
			if len(held) != 0 {
				t.Fatal("按鍵未釋放", held)
			}
		}
	}
	code, _ := rawkey.Code("darwin", "backspace")
	r, ok := virtualKeyRequest(1000 + 32768 + code)
	if !ok || r.Modifiers != 32 || r.Key != "backspace" {
		t.Fatalf("Fn 組合錯誤：%+v", r)
	}
	for _, action := range []int{-1, 42, 999, 66536, 99999} {
		if _, ok := virtualKeyRequest(action); ok {
			t.Fatalf("接受無效 action %d", action)
		}
	}
}

func TestVirtualKeyboardDoesNotBlockPointer(t *testing.T) {
	g := &game{systemShortcut: &systemShortcutDialog{request: shortcut.Request{Key: "virtual-keyboard"}}}
	if blocked, err := g.updateSystemShortcut(); blocked || err != nil {
		t.Fatal("虛擬鍵盤不應阻塞遠端滑鼠", blocked, err)
	}
	if !g.virtualKeyboardOpen() {
		t.Fatal("虛擬鍵盤應持續開啟")
	}
	g.systemShortcut.request.Key = "delete"
	if blocked, err := g.updateSystemShortcut(); !blocked || err != nil {
		t.Fatal("確認對話框仍應阻塞輸入", blocked, err)
	}
}
