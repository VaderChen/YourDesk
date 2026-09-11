package main

import (
	"fmt"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"image"
	"image/color"
	"math"
)

type viewMode int

const (
	viewShrink viewMode = iota // 只縮小，不放大。
	viewOriginal
	viewAuto
)

// 一次關閉，沿用既有結束與返回主畫面流程。
func (g *game) requestClose() error { return ebiten.Termination }

func (g *game) toolbarScale() float64 { return math.Max(1, ebiten.Monitor().DeviceScaleFactor()) }
func (g *game) toolbarHeight() int {
	if g.agentHeadless {
		return 0
	}
	if nativeTitlebarControls() {
		if nativeTitlebarOverlay() {
			return 0
		}
		// WebView 位於內容區頂端；繪圖與輸入共同扣除相同的實體像素高度。
		return int(math.Ceil(52 * g.toolbarScale()))
	}
	return int(math.Ceil(44 * g.toolbarScale()))
}
func (g *game) toolbarButton(i int) image.Rectangle {
	s := g.toolbarScale()
	x := float64(g.finalWidth) - (12+6*36+5*4)*s + float64(i)*40*s
	return image.Rect(int(x), int(4*s), int(x+36*s), int(40*s))
}

// 僅處理本機按鈕；整個按下／放開手勢都不轉送遠端。
func (g *game) updateToolbar() (bool, error) {
	nativeConfigureTitlebar()
	nativeSetTitlebarMode(int(g.mode))
	nativeSetQuality(g.quality)
	selected, count, pending := g.displayStatus()
	nativeSetDisplays(selected, count, pending)
	if action := nativeTitlebarAction(); action != 0 {
		if action >= 100 {
			g.selectDisplay(action - 100)
		}
		switch action {
		case 1:
			g.mode = viewOriginal
		case 2:
			g.mode = viewAuto
		case 3:
			g.mode = viewShrink
		case 4:
			g.toggleFullscreen()
		case 5, 13:
			return true, g.executeWindowShortcut(action)
		case 10, 11, 12:
			g.quality = action - 10
		case 6:
			g.nextDisplay()
		}
	}
	g.updateQuality()
	if nativeTitlebarControls() {
		x, y := ebiten.CursorPosition()
		px, py := g.finalTransform.Apply(float64(x), float64(y))
		return nativeTitlebarPopupOpen() || nativeTitlebarVisible() && px >= 0 && px < float64(g.finalWidth) && py >= 0 && py < 52*g.toolbarScale(), nil
	}
	x, y := ebiten.CursorPosition()
	px, py := g.finalTransform.Apply(float64(x), float64(y))
	hovered := -1
	if ebiten.IsFocused() {
		for i := 0; i < 6; i++ {
			if image.Pt(int(px), int(py)).In(g.toolbarButton(i)) {
				hovered = i
				break
			}
		}
	}
	g.toolbarHover = hovered
	down := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	captured := g.toolbarCapture != 0
	if down && !g.toolbarDown && py >= 0 && py < float64(g.toolbarHeight()) && ebiten.IsFocused() {
		g.toolbarCapture = hovered + 2 // 空白工具列同樣攔截手勢。
		captured = true
	}
	if !down && g.toolbarDown && g.toolbarCapture != 0 {
		if hovered >= 0 && hovered+2 == g.toolbarCapture && ebiten.IsFocused() {
			switch hovered {
			case 0:
				g.mode = viewOriginal
			case 1:
				g.mode = viewAuto
			case 2:
				g.mode = viewShrink
			case 3:
				g.toggleFullscreen()
			case 4:
				return true, g.requestClose()
			case 5:
				g.nextDisplay()
			}
		}
		g.toolbarCapture = 0
	}
	g.toolbarDown = down
	return captured || (py >= 0 && py < float64(g.toolbarHeight())), nil
}

func (g *game) viewport(iw, ih int) (float64, float64, float64, float64) {
	top := g.toolbarHeight()
	w, h := g.finalWidth, max(1, g.finalHeight-top)
	scale := math.Min(float64(max(1, w))/float64(max(1, iw)), float64(h)/float64(max(1, ih)))
	if !viewerFitWindow.Load() || g.isFullscreen() {
		switch g.mode {
		case viewOriginal:
			scale = 1
		case viewShrink:
			scale = math.Min(1, scale)
		}
	}
	vw, vh := math.Max(1, math.Floor(float64(max(1, iw))*scale)), math.Max(1, math.Floor(float64(max(1, ih))*scale))
	return math.Floor((float64(w) - vw) / 2), float64(top) + math.Floor((float64(h)-vh)/2), vw, vh
}

func (g *game) drawToolbar(screen viewerCanvas) {
	if nativeTitlebarControls() {
		return
	}
	w, h := g.finalWidth, g.toolbarHeight()
	if w < 1 {
		return
	}
	if g.toolbarImage == nil || g.toolbarImage.Bounds().Dx() != w || g.toolbarImage.Bounds().Dy() != h {
		if g.toolbarImage != nil {
			g.toolbarImage.Dispose()
		}
		g.toolbarImage = ebiten.NewImage(w, h)
	}
	dst := g.toolbarImage
	dst.Fill(color.RGBA{241, 246, 243, 255})
	s := float32(g.toolbarScale())
	selectedDisplay, displayCount, displayPending := g.displayStatus()
	vector.DrawFilledRect(dst, 0, float32(h)-1, float32(w), 1, color.RGBA{208, 220, 214, 255}, false)
	for i := 0; i < 6; i++ {
		rect := g.toolbarButton(i)
		x, y := float32(rect.Min.X), float32(rect.Min.Y)
		selected := (i == 0 && g.mode == viewOriginal) || (i == 1 && g.mode == viewAuto) || (i == 2 && g.mode == viewShrink)
		ink := color.RGBA{51, 83, 68, 255}
		if selected {
			vector.DrawFilledRect(dst, x, y, 36*s, 36*s, color.RGBA{33, 107, 80, 255}, true)
			ink = color.RGBA{255, 255, 255, 255}
		} else if i == g.toolbarHover {
			vector.DrawFilledRect(dst, x, y, 36*s, 36*s, color.RGBA{220, 231, 224, 255}, true)
		}
		if i == 5 && (displayCount < 2 || displayPending) {
			ink = color.RGBA{155, 163, 160, 255}
		}
		if i == 4 {
			ink = color.RGBA{201, 53, 53, 255}
		}
		line := func(a, b, c, d float32) { vector.StrokeLine(dst, x+a*s, y+b*s, x+c*s, y+d*s, 1.6*s, ink, true) }
		switch i {
		case 0: // 1:1
			line(8, 13, 11, 10)
			line(11, 10, 11, 26)
			line(8, 26, 14, 26)
			line(24, 13, 27, 10)
			line(27, 10, 27, 26)
			line(24, 26, 30, 26)
			line(18, 14, 18, 15)
			line(18, 22, 18, 23)
		case 1: // 自動填合：螢幕及對角向外箭頭。
			line(8, 8, 28, 8)
			line(28, 8, 28, 28)
			line(28, 28, 8, 28)
			line(8, 28, 8, 8)
			line(13, 23, 23, 13)
			line(18, 13, 23, 13)
			line(23, 13, 23, 18)
		case 2: // 只縮小：向內箭頭。
			line(8, 8, 15, 15)
			line(10, 15, 15, 15)
			line(15, 10, 15, 15)
			line(28, 28, 21, 21)
			line(21, 26, 21, 21)
			line(26, 21, 21, 21)
		case 3:
			if g.isFullscreen() {
				line(10, 14, 22, 14)
				line(22, 14, 22, 26)
				line(22, 26, 10, 26)
				line(10, 26, 10, 14)
				line(15, 10, 27, 10)
				line(27, 10, 27, 21)
			} else {
				line(8, 15, 8, 8)
				line(8, 8, 15, 8)
				line(21, 8, 28, 8)
				line(28, 8, 28, 15)
				line(8, 21, 8, 28)
				line(8, 28, 15, 28)
				line(21, 28, 28, 28)
				line(28, 28, 28, 21)
			}
		case 5:
			line(8, 9, 27, 9)
			line(27, 9, 27, 23)
			line(27, 23, 8, 23)
			line(8, 23, 8, 9)
			line(17, 23, 17, 27)
			line(12, 27, 23, 27)
		case 4:
			line(11, 11, 25, 25)
			line(25, 11, 11, 25)
		}
	}
	selected, count, pending := selectedDisplay, displayCount, displayPending
	label := "Display --"
	// 備援 GPU 工具列使用內建字型可顯示的數字標示。
	if count > 0 {
		label = fmt.Sprintf("Display %d/%d", selected+1, count)
	} else {
		label = "Display --"
	}
	if pending {
		label += " ..."
	}
	ebitenutil.DebugPrintAt(dst, label, int(12*s), int(14*s))
	screen.DrawImage(dst, &ebiten.DrawImageOptions{})
}
