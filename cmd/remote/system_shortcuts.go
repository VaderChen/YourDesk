package main

import (
	"github.com/hajimehoshi/ebiten/v2"
	"log/slog"
	"runtime"
	"yourdesk/internal/input"
	"yourdesk/internal/p2p"
	"yourdesk/internal/rawkey"
	"yourdesk/internal/shortcut"
)

const (
	shortcutLocal  = 40
	shortcutRemote = 41
	shortcutCancel = 42
)

type systemShortcutDialog struct {
	request  shortcut.Request
	peer     *p2p.Peer
	view     uint64
	display  int
	decision int
}

func (g *game) beginSystemShortcut(r shortcut.Request) {
	if g.systemShortcut != nil {
		return
	}
	// 取消未送出的輸入並釋放已送出的修飾鍵／滑鼠，避免對話框期間卡鍵。
	nativeSystemShortcutGuard(true)
	g.releaseRawKeys("選擇系統快捷鍵作用對象")
	clear(g.lastKeys)
	clear(g.lastButtons)
	captureRawKeys(false)
	g.rawActive = false
	g.mu.RLock()
	g.systemShortcut = &systemShortcutDialog{request: r, peer: g.peer, view: g.agentViewID, display: g.display}
	g.mu.RUnlock()
	remoteAvailable := g.peer != nil && g.peer.Connected() && g.controlEnabled && g.displayInputReady()
	if !nativeShowSystemShortcut(r.Label, r.Secure, remoteAvailable) {
		g.systemShortcut.decision = shortcutCancel
	}
}

func (g *game) cancelSystemShortcut() {
	if g.systemShortcut != nil {
		nativeCancelSystemShortcut()
		nativeSystemShortcutGuard(false)
		g.systemShortcut = nil
	}
}

// 對話框及放鍵等待期間只接收本機選擇，不把確認按鍵或滑鼠點擊送往遠端。
func (g *game) updateSystemShortcut() (bool, error) {
	d := g.systemShortcut
	if d == nil {
		return false, nil
	}
	captureRawKeys(false)
	if d.decision == 0 {
		return true, nil
	}
	if !nativeSystemShortcutReleased(d.request.Key, d.request.Modifiers) ||
		ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) ||
		ebiten.IsKeyPressed(ebiten.KeyEnter) || ebiten.IsKeyPressed(ebiten.KeyEscape) {
		return true, nil
	}
	g.systemShortcut = nil
	nativeSystemShortcutGuard(false)
	if d.decision == shortcutCancel || d.request.Secure {
		return true, nil
	}
	if d.decision == shortcutLocal {
		if d.request.LocalAction != 0 {
			return true, g.executeWindowShortcut(d.request.LocalAction)
		}
		// 目前非視窗生命週期的本機操作只有 Windows 工作管理員。
		for _, e := range d.request.Chord() {
			if err := (input.Native{}).RawKey(e); err != nil {
				for _, up := range d.request.Chord() {
					if !up.Down {
						_ = (input.Native{}).RawKey(up)
					}
				}
				slog.Warn("本機快捷鍵注入失敗", "error", err)
				break
			}
		}
		return true, nil
	}
	g.mu.RLock()
	sameView := g.agentViewID == d.view && g.display == d.display
	g.mu.RUnlock()
	if d.decision != shortcutRemote || g.peer != d.peer || g.peer == nil || !g.peer.Connected() ||
		!g.controlEnabled || !g.displayInputReady() || !sameView {
		return true, nil
	}
	for _, e := range d.request.Chord() {
		c := p2p.Control{Type: "raw-key", RawKey: &e}
		if !g.rawSupported.Load() {
			key := rawkey.Name(e.Platform, e.Code)
			switch key {
			case "leftshift":
				key = "shift"
			case "leftcontrol":
				key = "control"
			case "leftalt":
				key = "alt"
			case "leftsuper":
				key = "meta"
			}
			c = p2p.Control{Type: "key", Key: key, Down: e.Down}
		}
		if err := g.sendControl(c); err != nil {
			g.releaseRawKeys("快捷鍵傳送失敗")
			slog.Warn("遠端快捷鍵傳送失敗", "error", err)
			break
		}
	}
	return true, nil
}

// WebView 有焦點時同樣詢問；與點擊關閉視窗按鈕使用不同 action。
func (g *game) titlebarSystemShortcut(action int) {
	key, mods := "", uint64(0)
	switch action {
	case 50:
		key = "w"
		mods = 2
		if runtime.GOOS == "darwin" {
			mods = 8
		}
	case 51:
		key, mods = "q", 8
	case 52:
		key, mods = "f4", 4
	case 53:
		key, mods = "escape", 3
	case 55:
		key, mods = "delete", 6
	}
	if r, ok := shortcut.Match(runtime.GOOS, key, mods); ok {
		g.beginSystemShortcut(r)
	}
}
