package input

import (
	"image"
	"testing"
	"yourdesk/internal/agentvideo"
)

func TestRegionMouseCoordinates(t *testing.T) {
	region := agentvideo.Region{X: .25, Y: .25, Width: .5, Height: .5}
	controller := Native{Bounds: region.Bounds(image.Rect(-1920, -1080, 0, 0))}
	for _, tc := range []struct{ x, y, wantX, wantY float64 }{{0, 0, -1440, -810}, {1, 1, -481, -271}, {.5, .5, -960.5, -540.5}} {
		x, y := controller.point(tc.x, tc.y)
		if x != tc.wantX || y != tc.wantY {
			t.Fatalf("(%v,%v) => (%v,%v)", tc.x, tc.y, x, y)
		}
	}
	// 截圖即使由 1920×1080 縮至 1280×720，中心的相對座標仍為 0.5。
	x, y := controller.point(640.0/1280, 360.0/720)
	if x != -960.5 || y != -540.5 {
		t.Fatal("縮圖座標偏移")
	}
}
