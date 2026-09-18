package main

import (
	"github.com/hajimehoshi/ebiten/v2"
	"runtime"
	"yourdesk/internal/shortcut"
)

// 未連線或未啟用遠端控制時的本機視窗操作；C/V 留給既有剪貼簿流程。
func localWindowShortcut(platform, key string, mods uint64) int {
	if mods&1 != 0 {
		return 0
	}
	command := mods&2 != 0
	if platform == "darwin" {
		command = mods&8 != 0
	}
	if command && mods&4 == 0 && key == "w" {
		return 5
	}
	if platform == "darwin" && mods&8 != 0 && mods&(2|4) == 0 && key == "q" {
		return 13
	}
	if platform == "windows" && mods&4 != 0 && mods&(2|8) == 0 && key == "f4" {
		return 5
	}
	return 0
}
func (g *game) windowShortcut(raw bool) int {
	if g.localShortcut != 0 {
		action := g.localShortcut
		g.localShortcut = 0
		return action
	}
	if (raw && g.rawActive) || !ebiten.IsFocused() {
		return 0
	}
	var mods uint64
	if ebiten.IsKeyPressed(ebiten.KeyShift) {
		mods |= 1
	}
	if ebiten.IsKeyPressed(ebiten.KeyControl) {
		mods |= 2
	}
	if ebiten.IsKeyPressed(ebiten.KeyAlt) {
		mods |= 4
	}
	if ebiten.IsKeyPressed(ebiten.KeyMeta) {
		mods |= 8
	}
	for _, key := range []ebiten.Key{ebiten.KeyW, ebiten.KeyQ, ebiten.KeyF4, ebiten.KeyDelete, ebiten.KeyEscape} {
		if ebiten.IsKeyPressed(key) {
			if g.peer != nil && g.peer.Connected() && g.controlEnabled && g.displayInputReady() {
				if request, ok := shortcut.Match(runtime.GOOS, commonKeys[key], mods); ok {
					g.beginSystemShortcut(request)
					return 0
				}
			}
			if action := localWindowShortcut(runtime.GOOS, commonKeys[key], mods); action != 0 {
				return action
			}
		}
	}
	return 0
}
func (g *game) executeWindowShortcut(action int) error {
	g.releaseRawKeys("關閉 遠端顯示")
	if action == 13 {
		emitUIEvent("quit-application", "")
	}
	return g.requestClose()
}
