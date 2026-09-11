package main

import (
	"math"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

type windowFitState struct {
	enabled bool
	key     [5]int
	changed time.Time
	handled bool
	axis    int
}

// 只修正一般視窗的尺寸，不裁切遠端畫面；短暫等待可避免與使用者拖曳互相拉扯。
// Ebitengine 視窗使用邏輯尺寸，toolbarHeight 則為實體像素，必須先換算。
func (g *game) updateWindowFit() {
	p := &g.windowFit
	enabled := viewerFitWindow.Load()
	if enabled != p.enabled {
		*p = windowFitState{enabled: enabled}
		if !enabled {
			ebiten.SetWindowSizeLimits(640, 270, -1, -1)
		}
	}
	if !enabled || g.isFullscreen() || nativeFullscreenTransitioning() || g.restorePreferences {
		p.handled = false
		p.changed = time.Now()
		return
	}
	g.mu.Lock()
	iw, ih := 0, 0
	if g.frame != nil {
		iw, ih = g.frame.Bounds().Dx(), g.frame.Bounds().Dy()
	}
	g.mu.Unlock()
	if iw <= 0 || ih <= 0 {
		return
	}
	w, h := ebiten.WindowSize()
	top := int(math.Round(float64(g.toolbarHeight()) / g.toolbarScale()))
	key := [5]int{w, h, iw, ih, top}
	if key != p.key {
		p.axis = 0
		if p.key[2] == iw && p.key[3] == ih && p.key[4] == top {
			if w != p.key[0] && h == p.key[1] {
				p.axis = 1
			}
			if h != p.key[1] && w == p.key[0] {
				p.axis = 2
			}
		}
		p.key = key
		p.changed = time.Now()
		p.handled = false
		return
	}
	if p.handled || time.Since(p.changed) < 200*time.Millisecond {
		return
	}
	p.handled = true
	// 縮進使用者選定的範圍，保留完整畫面；最小視窗也採相同比例。
	minimum := math.Min(640/float64(iw), float64(max(1, 270-top))/float64(ih))
	minW, minH := max(1, int(math.Round(float64(iw)*minimum))), max(top+1, int(math.Round(float64(ih)*minimum))+top)
	ebiten.SetWindowSizeLimits(minW, minH, -1, -1)
	scale := math.Max(minimum, math.Min(float64(w)/float64(iw), float64(max(1, h-top))/float64(ih)))
	if p.axis == 1 {
		scale = math.Max(minimum, float64(w)/float64(iw))
	}
	if p.axis == 2 {
		scale = math.Max(minimum, float64(max(1, h-top))/float64(ih))
	}
	nw, nh := int(math.Round(float64(iw)*scale)), int(math.Round(float64(ih)*scale))+top
	if absInt(w-nw) > 1 || absInt(h-nh) > 1 {
		ebiten.SetWindowSize(max(minW, nw), max(minH, nh))
	}
}
