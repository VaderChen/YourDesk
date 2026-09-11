package main

import (
	"github.com/hajimehoshi/ebiten/v2"
	"yourdesk/internal/p2p"
	"yourdesk/internal/rawkey"
)

func (g *game) releaseRawKeys(reason string) {
	if g.peer != nil {
		g.clipboard.CancelKeys(reason)
		_ = g.peer.SendControl(p2p.Control{Type: "raw-key-reset"})
	}
	clear(g.rawHeld)
}
func (g *game) updateRawKeys() bool {
	supported := g.rawSupported.Load() && rawKeyboardAvailable()
	enabled := supported && !nativeTitlebarPopupOpen() && ebiten.IsFocused() && g.controlEnabled && g.displayInputReady()
	events := captureRawKeys(enabled)
	if enabled && !g.rawActive {
		// 能力協商完成時，釋放舊協定留下的按鍵狀態。
		for key, down := range g.lastKeys {
			if down {
				_ = g.sendControl(p2p.Control{Type: "key", Key: commonKeys[key], Down: false})
			}
		}
		clear(g.lastKeys)
	}
	if g.rawActive && !enabled {
		g.releaseRawKeys("遠端顯示 失去焦點或遠端輸入暫停")
	}
	g.rawActive = enabled
	if !enabled {
		// 恢復已保存的暫停控制狀態時，仍可用 F12 重新啟用。
		return supported && g.controlEnabled
	}
	for _, event := range events {
		if event.Reset {
			g.releaseRawKeys("本機鍵盤事件重設")
			continue
		}
		if event.Down && !event.Repeat {
			if action := localWindowShortcut(event.Platform, rawkey.Name(event.Platform, event.Code), event.Modifiers); action != 0 {
				g.localShortcut = action
				g.releaseRawKeys("本機視窗快捷鍵")
				break
			}
		}
		// 貼上等待獨立剪貼簿通道的接收確認，保留按鍵順序。
		if event.Down && !event.Repeat && rawkey.Name(event.Platform, event.Code) == "v" && event.Modifiers&(2|8) != 0 && g.clipboard != nil {
			g.clipboard.Poll(true)
		}
		copy := event
		if err := g.sendControl(p2p.Control{Type: "raw-key", RawKey: &copy}); err == nil {
			if event.Down {
				g.rawHeld[event.Code] = event
			} else {
				delete(g.rawHeld, event.Code)
			}
		}
	}
	return supported
}
