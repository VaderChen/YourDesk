package main

import (
	"encoding/json"
	"fmt"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"image"
	"image/color"
	"math"
	"time"
	"yourdesk/internal/agentremote"
	"yourdesk/internal/agentvideo"
)

// 所有編輯狀態僅由 Viewer Update 執行緒存取。
type cropEditor struct {
	composite                       *ebiten.Image
	confirming, draining            bool
	waitUntil                       time.Time
	publishedState                  int
	publishedMessage                string
	streamSize                      image.Point
	editing, pending, waiting, down bool
	active                          bool
	purpose, message                string
	original                        agentvideo.Region
	rect, start                     image.Rectangle
	size                            image.Point
	anchor                          image.Point
	handle                          int // 0 新框，1 移動，2..9 八個調整點。
	overlay                         *ebiten.Image
}

func (c *cropEditor) blockInput() bool {
	return c.editing || c.pending || c.waiting || c.confirming || c.draining
}
func (c *cropEditor) status() int {
	if c.pending || c.waiting {
		return 2
	}
	if c.active && !c.editing {
		return 3
	}
	if c.editing {
		return 1
	}
	return 0
}

// 寬高取最接近的 8 倍數，並將框移回邊界內；小於 8 的來源保留原尺寸。
func alignedCrop(r image.Rectangle, size image.Point) image.Rectangle {
	if size.X < 1 || size.Y < 1 {
		return image.Rectangle{}
	}
	r = r.Canon()
	align := func(v, limit int) int {
		if limit < 8 {
			return limit
		}
		return min(max(8, ((v+4)/8)*8), limit/8*8)
	}
	w, h := align(r.Dx(), size.X), align(r.Dy(), size.Y)
	x, y := max(0, min(r.Min.X, size.X-w)), max(0, min(r.Min.Y, size.Y-h))
	return image.Rect(x, y, x+w, y+h)
}
func cropRegion(r image.Rectangle, size image.Point) agentvideo.Region {
	// 微小向內偏移避免遠端 floor/ceil 的浮點誤差產生多一像素。
	const epsilon = 1e-7
	return agentvideo.Region{X: (float64(r.Min.X) + epsilon) / float64(size.X), Y: (float64(r.Min.Y) + epsilon) / float64(size.Y), Width: (float64(r.Dx()) - 2*epsilon) / float64(size.X), Height: (float64(r.Dy()) - 2*epsilon) / float64(size.Y)}
}
func (g *game) cropRequest(region agentvideo.Region, purpose string) {
	g.crop.pending = true
	g.crop.purpose = purpose
	g.crop.message = "正在切換串流區域…"
	request := agentvideo.Request{Mode: "streaming", Region: &region}
	if region == agentvideo.Full() {
		request.Region = nil
		request.FullScreen = true
	}
	params, _ := json.Marshal(request)
	g.dispatchVideo(agentremote.Request{Action: "video.stream", Params: params, Expires: time.Now().Add(10 * time.Second).UnixMilli()}, true)
}
func (g *game) toggleCrop() {
	c := &g.crop
	if c.pending || c.waiting || c.confirming || g.agentVideoBusy {
		return
	}
	if c.active && !c.editing {
		c.confirming = true
		g.releaseRawKeys("確認復原裁切")
		nativeConfirmCrop()
		return
	}
	if c.editing {
		g.cropRequest(cropRegion(c.rect, c.size), "apply")
		return
	}
	if g.peer == nil || !g.peer.Connected() || !g.displayInputReady() || g.displayedWidth < 8 || g.displayedHeight < 8 {
		c.message = "等待遠端完整影格"
		return
	}
	if !g.peer.SupportsCommand("video.stream") {
		c.message = "遠端版本不支援區域串流，請更新 Host"
		return
	}
	g.releaseRawKeys("調整裁切框")
	clear(g.lastButtons)
	clear(g.lastKeys)
	g.mu.Lock()
	c.original = g.agentRegion
	g.mu.Unlock()
	if c.original.Validate() != nil {
		c.original = agentvideo.Full()
	}
	c.editing = true
	c.message = "拖曳框內移動、邊角調整尺寸；再次按裁切套用；Esc 取消"
	if c.original != agentvideo.Full() {
		g.cropRequest(agentvideo.Full(), "edit")
		return
	}
	g.initializeCrop()
}
func (g *game) initializeCrop() {
	c := &g.crop
	c.streamSize = image.Pt(g.displayedWidth, g.displayedHeight)
	c.size = c.streamSize
	if r := g.remoteEnhancement.Load(); r != nil && r.StreamWidth == c.streamSize.X && r.StreamHeight == c.streamSize.Y && r.SourceWidth > 0 && r.SourceHeight > 0 {
		c.size = image.Pt(r.SourceWidth, r.SourceHeight)
	}
	if c.original == agentvideo.Full() {
		c.rect = defaultCrop(c.size)
	} else {
		c.rect = alignedCrop(c.original.Bounds(image.Rectangle{Max: c.size}), c.size)
	}
	c.down = false
}
func (g *game) resetCrop() {
	if g.crop.pending || g.crop.waiting || g.agentVideoBusy {
		return
	}
	g.cropRequest(agentvideo.Full(), "reset")
}
func (g *game) finishCrop(result agentVideoResult) {
	c := &g.crop
	c.pending = false
	if result.out.Error != "" {
		c.message = result.out.Error
		return
	}
	c.active = result.state.Region != agentvideo.Full()
	c.waiting = true
	c.waitUntil = time.Now().Add(10 * time.Second)
	c.draining = true
	if c.purpose != "edit" {
		c.editing = false
	}
	c.message = "等待新區域的完整影格…"
}
func (g *game) updateCrop(toolbar bool) bool {
	c := &g.crop
	if c.waiting && g.displayInputReady() {
		c.waiting = false
		if c.purpose == "edit" {
			g.initializeCrop()
		}
		c.message = "裁切：拖曳調整，再按一次套用"
		if c.active {
			c.message = "復原：恢復全畫面串流"
		}
		if c.editing {
			c.message = "拖曳框內移動、邊角調整尺寸；再次按裁切套用；Esc 取消"
		}
	}
	if c.waiting && time.Now().After(c.waitUntil) {
		c.waiting = false
		c.editing = false
		c.message = "尚未收到新區域影格，可按復原重試"
	}
	if !c.blockInput() {
		return false
	}
	if c.confirming && !nativeTitlebarControls() {
		if ebiten.IsKeyPressed(ebiten.KeyEscape) {
			c.confirming = false
			c.draining = true
		}
		if ebiten.IsKeyPressed(ebiten.KeyEnter) {
			c.confirming = false
			g.resetCrop()
		}
		return true
	}
	if c.pending || c.waiting || c.confirming {
		return true
	}
	if c.draining && !c.editing {
		if !cropKeyPressed() && !ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) && !ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight) && !ebiten.IsMouseButtonPressed(ebiten.MouseButtonMiddle) {
			c.draining = false
		}
		return true
	}
	if !ebiten.IsFocused() {
		c.down = false
		return true
	}
	if ebiten.IsKeyPressed(ebiten.KeyEscape) {
		g.cropRequest(c.original, "cancel")
		return true
	}

	if c.streamSize != image.Pt(g.displayedWidth, g.displayedHeight) {
		g.initializeCrop()
	}
	x, y := ebiten.CursorPosition()
	px, py := g.finalTransform.Apply(float64(x), float64(y))
	ox, oy, w, h := g.viewport(g.displayedWidth, g.displayedHeight)
	p := image.Pt(int(math.Round((px-ox)/w*float64(c.size.X))), int(math.Round((py-oy)/h*float64(c.size.Y))))
	p.X = max(0, min(c.size.X, p.X))
	p.Y = max(0, min(c.size.Y, p.Y))
	down := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	if down && !c.down && !toolbar && py >= float64(g.toolbarHeight()) && px >= ox && px <= ox+w && py >= oy && py <= oy+h {
		c.anchor = p
		c.start = c.rect
		c.handle = 0
		tolerance := max(4, int(10*g.toolbarScale()*float64(c.size.X)/w))
		for i, q := range cropHandles(c.rect) {
			if absInt(p.X-q.X) <= tolerance && absInt(p.Y-q.Y) <= tolerance {
				c.handle = i + 2
				break
			}
		}
		if c.handle == 0 && p.In(c.rect) {
			c.handle = 1
		}
		c.down = true
	}
	if down && c.down {
		r := c.start
		switch c.handle {
		case 0:
			r = image.Rectangle{Min: c.anchor, Max: p}.Canon()
		case 1:
			r = r.Add(p.Sub(c.anchor))
		default:
			i := c.handle - 2
			if i == 0 || i == 6 || i == 7 {
				r.Min.X = p.X
			}
			if i == 2 || i == 3 || i == 4 {
				r.Max.X = p.X
			}
			if i == 0 || i == 1 || i == 2 {
				r.Min.Y = p.Y
			}
			if i == 4 || i == 5 || i == 6 {
				r.Max.Y = p.Y
			}
		}
		c.rect = alignedCrop(r, c.size)
	}
	if !down {
		c.down = false
	}
	return true
}
func cropHandles(r image.Rectangle) []image.Point {
	x, y := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2
	return []image.Point{r.Min, {x, r.Min.Y}, {r.Max.X, r.Min.Y}, {r.Max.X, y}, r.Max, {x, r.Max.Y}, {r.Min.X, r.Max.Y}, {r.Min.X, y}}
}
func (g *game) drawCrop(screen viewerCanvas) {
	c := &g.crop
	if !c.visible() {
		return
	}
	b := screen.Bounds()
	if c.overlay == nil || c.overlay.Bounds() != b {
		if c.overlay != nil {
			c.overlay.Dispose()
		}
		c.overlay = ebiten.NewImage(b.Dx(), b.Dy())
	}
	dst := c.overlay
	dst.Clear()
	if c.confirming {
		vector.DrawFilledRect(dst, 0, 0, float32(b.Dx()), float32(b.Dy()), color.RGBA{0, 0, 0, 180}, false)
		ebitenutil.DebugPrintAt(dst, "Restore full-screen streaming?\nEnter: Restore     Esc: Cancel", max(8, b.Dx()/2-130), b.Dy()/2)
		screen.DrawImage(dst, &ebiten.DrawImageOptions{})
		return
	}
	ox, oy, w, h := g.viewport(g.displayedWidth, g.displayedHeight)
	rect := func(x, y, a, b float64, ink color.Color) {
		vector.DrawFilledRect(dst, float32(x), float32(y), float32(math.Max(0, a)), float32(math.Max(0, b)), ink, false)
	}
	x := ox + float64(c.rect.Min.X)/float64(c.size.X)*w
	y := oy + float64(c.rect.Min.Y)/float64(c.size.Y)*h
	rw := float64(c.rect.Dx()) / float64(c.size.X) * w
	rh := float64(c.rect.Dy()) / float64(c.size.Y) * h
	shade := color.RGBA{0, 0, 0, 140}
	ink := color.RGBA{75, 230, 165, 255}
	rect(ox, oy, w, y-oy, shade)
	rect(ox, y+rh, w, oy+h-y-rh, shade)
	rect(ox, y, x-ox, rh, shade)
	rect(x+rw, y, ox+w-x-rw, rh, shade)
	vector.StrokeRect(dst, float32(x), float32(y), float32(rw), float32(rh), 2, ink, false)
	for _, p := range cropHandles(c.rect) {
		a := ox + float64(p.X)/float64(c.size.X)*w
		b := oy + float64(p.Y)/float64(c.size.Y)*h
		rect(a-5, b-5, 10, 10, ink)
	}
	label := fmt.Sprintf("%d x %d | Crop: apply  Esc: cancel", c.rect.Dx(), c.rect.Dy())
	ly := math.Max(float64(g.toolbarHeight())+8, y-24)
	rect(math.Max(0, x), ly, float64(len(label)*6+8), 20, color.RGBA{0, 0, 0, 220})
	ebitenutil.DebugPrintAt(dst, label, int(math.Max(0, x))+4, int(ly)+2)
	screen.DrawImage(dst, &ebiten.DrawImageOptions{})
}

func cropKeyPressed() bool {
	for key := ebiten.Key(0); key <= ebiten.KeyMax; key++ {
		if ebiten.IsKeyPressed(key) {
			return true
		}
	}
	return false
}

// 先對齊寬高，再置中，避免取整數後偏到任一側。
func defaultCrop(size image.Point) image.Rectangle {
	r := alignedCrop(image.Rect(0, 0, size.X/2, size.Y/2), size)
	return r.Add(image.Pt((size.X-r.Dx())/2, (size.Y-r.Dy())/2))
}
func (c *cropEditor) visible() bool {
	return (c.editing || c.confirming && !nativeTitlebarControls()) && !c.pending && !c.waiting && c.size.X > 0
}
