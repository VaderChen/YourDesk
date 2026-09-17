package main

import (
	"math"
	"testing"
)

func TestNativePointerUsesCanvasCoordinates(t *testing.T) {
	for _, tc := range []struct {
		name         string
		nx, ny       float64
		w, h         int
		valid        bool
		wantX, wantY float64
	}{
		{"retina", .25, .75, 2000, 1000, true, 500, 750},
		{"outside", -0.1, 1.2, 1000, 500, true, -100, 600},
		{"not ready", .5, .5, 1000, 500, false, 10, 20},
		{"no canvas", .5, .5, 0, 500, true, 10, 20},
		{"invalid", math.NaN(), .5, 1000, 500, true, 10, 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			x, y := canvasPointer(10, 20, tc.nx, tc.ny, tc.w, tc.h, tc.valid)
			if x != tc.wantX || y != tc.wantY {
				t.Fatalf("got (%v,%v)", x, y)
			}
		})
	}
	// 引擎座標不變時，觸控板的原生位置仍更新；微小位移也不受閒置門檻限制。
	x1, _ := canvasPointer(10, 20, .5, .5, 1000, 500, true)
	x2, _ := canvasPointer(10, 20, .501, .5, 1000, 500, true)
	if x2-x1 != 1 {
		t.Fatal("small native motion ignored")
	}
}
