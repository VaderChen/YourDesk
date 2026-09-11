package main

import (
	"runtime"
	"yourdesk/internal/p2p"
)

// 控制通道與影像通道非同步；切換後只接收目標螢幕的完整影格。
func (g *game) receiveDisplays(c p2p.Control) {
	if c.Type != "displays" || c.Display == nil || c.DisplayCount < 0 {
		return
	}
	selected := *c.Display
	if selected < 0 || (c.DisplayCount > 0 && selected >= c.DisplayCount) {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if c.DisplayRequest < g.displayRequest {
		return
	}
	if !g.displayKnown || selected != g.display {
		g.displayPending = g.frame == nil || g.frameDisplay != selected
	}
	g.displayKnown, g.display, g.displayCount = true, selected, c.DisplayCount
}

func (g *game) nextDisplay()            { g.changeDisplay(-1) }
func (g *game) selectDisplay(index int) { g.changeDisplay(index) }

func (g *game) changeDisplay(index int) {
	if g.peer == nil {
		return
	}
	g.mu.Lock()
	if !g.displayKnown || g.displayCount < 2 || g.displayPending || index >= g.displayCount || index == g.display {
		g.mu.Unlock()
		return
	}
	g.displayRequest++
	request := g.displayRequest
	previous := g.display
	kind := "display-select"
	if index < 0 {
		index = (g.display + 1) % g.displayCount
		kind = "display-next"
	}
	g.display = index
	g.displayPending = true
	g.mu.Unlock()
	if err := g.peer.SendControl(p2p.Control{Type: kind, Display: &index, DisplayRequest: request}); err != nil {
		g.mu.Lock()
		g.displayRequest--
		g.display = previous
		g.displayPending = false
		g.mu.Unlock()
	}
	// Host 在切換時會釋放按鍵，本機同步忘記舊狀態。
	clear(g.lastButtons)
	clear(g.lastKeys)
}

func (g *game) displayInputReady() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return !g.displayKnown || (!g.displayPending && g.displayCount > 0 && g.displayedDisplay == g.display)
}

func (g *game) sendControl(c p2p.Control) error {
	c.Platform = runtime.GOOS
	c.DisableMapping = g.disableKeyMapping
	if c.RawKey != nil {
		copy := *c.RawKey
		copy.DisableMapping = g.disableKeyMapping
		c.RawKey = &copy
	}
	g.mu.RLock()
	selected, known := g.displayedDisplay, g.displayKnown
	g.mu.RUnlock()
	if known {
		c.Display = &selected
	}
	return g.clipboard.SendControl(c)
}

func (g *game) displayStatus() (int, int, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.display, g.displayCount, g.displayPending
}
