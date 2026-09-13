package agentvideo

import (
	"image"
	"image/color"
	"math"
	"testing"
)

func TestViewCoordinates(t *testing.T) {
	r := Region{X: .25, Y: .25, Width: .5, Height: .5}
	for _, tc := range []struct{ input, want image.Rectangle }{
		{image.Rect(-1920, -1080, 0, 0), image.Rect(-1440, -810, -480, -270)},
		{image.Rect(0, 0, 3840, 2160), image.Rect(960, 540, 2880, 1620)},
		{image.Rect(0, 0, 1, 1), image.Rect(0, 0, 1, 1)},
	} {
		if got := r.Bounds(tc.input); got != tc.want {
			t.Fatalf("%v => %v, want %v", tc.input, got, tc.want)
		}
	}
	source := image.NewRGBA(image.Rect(10, 20, 18, 28))
	source.Set(12, 22, color.RGBA{255, 0, 0, 255})
	crop := r.Crop(source)
	if crop.Bounds() != image.Rect(0, 0, 4, 4) || crop.RGBAAt(0, 0).R != 255 {
		t.Fatal("裁切未正規化原點")
	}
	for _, bad := range []Region{{Width: 0, Height: 1}, {X: .9, Width: .2, Height: 1}, {Y: math.NaN(), Width: 1, Height: 1}, {Width: math.Inf(1), Height: 1}, {X: -.1, Width: 1, Height: 1}} {
		if bad.Validate() == nil {
			t.Fatalf("接受無效區域 %+v", bad)
		}
	}
}
