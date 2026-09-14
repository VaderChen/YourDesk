package main

import (
	"github.com/hajimehoshi/ebiten/v2"
	"image"
	"image/color"
	"testing"
	"yourdesk/internal/agentremote"
	"yourdesk/internal/agentvideo"
	"yourdesk/internal/p2p"
)

func TestCropAlignmentAndRemoteBounds(t *testing.T) {
	for _, size := range []image.Point{{1920, 1080}, {3024, 1964}, {1512, 982}, {7, 5}, {8192, 4320}} {
		for _, r := range []image.Rectangle{image.Rect(0, 0, size.X, size.Y), image.Rect(119, 37, 558, 382), image.Rect(-100, -30, 300, 211), image.Rect(size.X-10, size.Y-5, size.X+200, size.Y+50), {Min: image.Pt(490, 303), Max: image.Pt(49, 101)}} {
			box := alignedCrop(r, size)
			if box.Empty() || !box.In(image.Rectangle{Max: size}) || size.X >= 8 && box.Dx()%8 != 0 || size.Y >= 8 && box.Dy()%8 != 0 {
				t.Fatalf("尺寸 %v，裁切框 %v", size, box)
			}
			region := cropRegion(box, size)
			if err := region.Validate(); err != nil {
				t.Fatal(err)
			}
			if got := region.Bounds(image.Rectangle{Max: size}); got != box {
				t.Fatalf("遠端收到的框 %v != %v", got, box)
			}
			// Retina 螢幕邏輯座標含負原點，中心必須落在同一選取區域。
			desktop := image.Rect(-size.X/2, -100, 0, size.Y/2-100)
			actual := region.Bounds(desktop)
			expectedX := desktop.Min.X + (box.Min.X+box.Dx()/2)/2
			if absInt((actual.Min.X+actual.Max.X)/2-expectedX) > 1 {
				t.Fatalf("Retina 座標偏移：%v", actual)
			}
		}
	}
}
func TestCropReplyAndInputBarrier(t *testing.T) {
	g := &game{}
	g.crop.editing = true
	g.crop.pending = true
	g.crop.purpose = "apply"
	region := agentvideo.Region{X: .25, Y: .25, Width: .5, Height: .5}
	g.finishCrop(agentVideoResult{state: agentvideo.State{Region: region}})
	if g.crop.editing || !g.crop.waiting || !g.crop.active || !g.crop.blockInput() {
		t.Fatal("確認回覆後應等待新影格並封鎖輸入")
	}
	g.crop.waiting = false
	if g.crop.status() != 3 {
		t.Fatal("裁切後應為復原按鈕")
	}
	g.crop.pending = true
	g.crop.purpose = "reset"
	g.finishCrop(agentVideoResult{out: agentremote.Response{Error: "測試逾時"}})
	if !g.crop.active || g.crop.pending || g.crop.message != "測試逾時" {
		t.Fatal("復原失敗不得假裝已恢復全畫面")
	}
	g.finishCrop(agentVideoResult{state: agentvideo.State{Region: agentvideo.Full()}})
	if g.crop.active || !g.crop.waiting {
		t.Fatal("復原成功仍須等待全畫面影格")
	}
}

func TestCropUsesSourcePixelsWhenStreamScaled(t *testing.T) {
	g := &game{displayedWidth: 1280, displayedHeight: 720}
	g.crop.original = agentvideo.Full()
	g.remoteEnhancement.Store(&p2p.EnhancementReport{SourceWidth: 3024, SourceHeight: 1964, StreamWidth: 1280, StreamHeight: 720})
	g.initializeCrop()
	if g.crop.size != image.Pt(3024, 1964) || g.crop.rect.Dx() != 1512 || g.crop.rect.Dy() != 984 {
		t.Fatalf("應以來源尺寸對齊裁切框：%+v", g.crop)
	}
}

func TestDefaultCropCenteredHalf(t *testing.T) {
	for _, size := range []image.Point{{1920, 1080}, {3024, 1964}, {1512, 982}} {
		r := defaultCrop(size)
		if absInt(r.Dx()-size.X/2) > 4 || absInt(r.Dy()-size.Y/2) > 4 || r.Dx()%8 != 0 || r.Dy()%8 != 0 {
			t.Fatalf("預設尺寸未對齊：%v %v", size, r)
		}
		if absInt(r.Min.X-(size.X-r.Max.X)) > 1 || absInt(r.Min.Y-(size.Y-r.Max.Y)) > 1 {
			t.Fatalf("未置中：%v", r)
		}
	}
}

// Metal 的 screen pass 不可在影像與透明圖層之間切換至離屏繪圖。
// 驗證最終畫布只收到一次已合成的影像，避免背景再次被清除。
type cropFinalScreenSmoke struct {
	ebiten.FinalScreen
	images []*ebiten.Image
	fills  int
}

func (s *cropFinalScreenSmoke) Bounds() image.Rectangle { return image.Rect(0, 0, 640, 480) }
func (s *cropFinalScreenSmoke) Fill(color.Color)        { s.fills++ }
func (s *cropFinalScreenSmoke) DrawImage(img *ebiten.Image, _ *ebiten.DrawImageOptions) {
	s.images = append(s.images, img)
}
func TestCropFinalScreenSingleSubmission(t *testing.T) {
	g := &game{agentHeadless: true, ml: &mlWorker{}, frame: image.NewRGBA(image.Rect(0, 0, 640, 480)), dirty: true, displayedWidth: 640, displayedHeight: 480}
	g.crop.editing = true
	g.crop.original = agentvideo.Full()
	g.initializeCrop()
	screen := &cropFinalScreenSmoke{}
	g.DrawFinalScreen(screen, nil, ebiten.GeoM{})
	if len(screen.images) != 1 || screen.images[0] != g.crop.composite || screen.fills != 0 || g.texture == nil || g.crop.overlay == nil {
		t.Fatal("遠端影像與裁切框未在提交視窗前完成合成")
	}
	g.texture.Dispose()
	g.crop.overlay.Dispose()
	g.crop.composite.Dispose()
}
