package main

import (
	"encoding/json"
	"runtime"
	"yourdesk/internal/p2p"
	"yourdesk/internal/rawkey"
	"yourdesk/internal/shortcut"
)

// 每次點擊是一組完整按下／放開；修飾鍵只鎖定在 UI，不留在遠端。
func (g *game) showVirtualKeyboard() {
	if !g.rawSupported.Load() {
		emitUIEvent("error", "遠端目前無法接受輸入。")
		return
	}
	if g.systemShortcut != nil {
		return
	}
	platform, _ := g.remoteKeyboardPlatform.Load().(string)
	if platform != "darwin" && platform != "windows" {
		platform = runtime.GOOS
	}
	codes := map[string]map[string]int{}
	for _, p := range []string{"darwin", "windows"} {
		codes[p] = map[string]int{}
		for code := 0; code < 512; code++ {
			if name := rawkey.Name(p, code); name != "" {
				codes[p][name] = code
			}
		}
	}
	payload, _ := json.Marshal(map[string]any{"platform": platform, "codes": codes})
	nativeSystemShortcutGuard(true)
	g.releaseRawKeys("開啟虛擬鍵盤")
	clear(g.lastKeys)
	clear(g.lastButtons)
	captureRawKeys(false)
	g.rawActive = false
	g.mu.RLock()
	g.systemShortcut = &systemShortcutDialog{request: shortcut.Request{Key: "virtual-keyboard"}, peer: g.peer, view: g.agentViewID, display: g.display}
	g.mu.RUnlock()
	if !nativeShowVirtualKeyboard(string(payload)) {
		g.systemShortcut.decision = shortcutCancel
	}
}

func virtualKeyRequest(action int) (shortcut.Request, bool) {
	n := action - 1000
	if n < 0 || n >= 65536 {
		return shortcut.Request{}, false
	}
	caps := uint64(0)
	if n >= 32768 {
		caps |= 32
		n -= 32768
	}
	if n >= 16384 {
		caps |= 16
		n -= 16384
	}
	platform := "darwin"
	if n >= 8192 {
		platform = "windows"
		n -= 8192
	}
	key := rawkey.Name(platform, n%512)
	if key == "" {
		return shortcut.Request{}, false
	}
	return shortcut.Request{Platform: platform, Key: key, Modifiers: uint64(n/512) | caps}, true
}

func (g *game) sendVirtualKey(action int) {
	d := g.systemShortcut
	if d == nil || d.request.Key != "virtual-keyboard" || d.decision != 0 {
		return
	}
	g.mu.RLock()
	same := g.agentViewID == d.view && g.display == d.display
	g.mu.RUnlock()
	if !same || g.peer == nil || g.peer != d.peer || !g.peer.Connected() || !g.controlEnabled || !g.displayInputReady() {
		return
	}
	r, ok := virtualKeyRequest(action)
	if !ok {
		return
	}
	if r.Key == "delete" && r.Modifiers&15 == 6 {
		g.sendSecureAttention()
		return
	}
	if r.Key == "v" && r.Modifiers&(2|8) != 0 && g.clipboard != nil {
		g.clipboard.Poll(true)
	}
	events := r.Chord()
	for i, e := range events {
		e.Modifiers |= r.Modifiers & 48
		if i == len(events)-1 {
			e.Modifiers &^= 32
		}
		c := p2p.Control{Type: "raw-key", RawKey: &e}
		if err := g.sendControl(c); err != nil {
			g.releaseRawKeys("虛擬鍵盤傳送失敗")
			emitUIEvent("error", err.Error())
			return
		}
	}
}

func (g *game) virtualKeyboardOpen() bool {
	return g.systemShortcut != nil && g.systemShortcut.request.Key == "virtual-keyboard" && g.systemShortcut.decision == 0
}
