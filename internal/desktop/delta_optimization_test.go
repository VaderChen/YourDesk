package desktop

import (
	"errors"
	"image"
	"image/color"
	"testing"
)

type failOnceJPEG struct{ fail bool }

func (e *failOnceJPEG) Encode(image.Image, int) ([]byte, error) {
	if e.fail {
		e.fail = false
		return nil, errors.New("合成錯誤")
	}
	return []byte{1}, nil
}
func TestDeltaOptimizedOddPixelAndRetry(t *testing.T) {
	for _, budget := range []int{0, 64 << 20} {
		src := image.NewRGBA(image.Rect(11, 13, 267, 269))
		codec := &failOnceJPEG{}
		e := DeltaEncoder{Encoder: codec, CompareBytes: budget}
		if _, err := e.Encode(src, false); err != nil {
			t.Fatal(err)
		}
		src.SetRGBA(12, 14, color.RGBA{R: 255, A: 255})
		codec.fail = true
		if _, err := e.Encode(src, false); err == nil {
			t.Fatal("未注入錯誤")
		}
		patches, err := e.Encode(src, false)
		if err != nil || len(patches) != 1 || patches[0].Keyframe {
			t.Fatalf("未保留失敗區塊：%v %v", patches, err)
		}
		if patches[0].X != 0 || patches[0].Y != 0 {
			t.Fatal("區塊座標錯誤")
		}
		if patches, err = e.Encode(src, false); err != nil || len(patches) != 0 {
			t.Fatal("未變畫面仍送出")
		}
	}
}
func TestPatchImageSharesStride(t *testing.T) {
	src := image.NewRGBA(image.Rect(5, 7, 261, 263))
	p := patchImage(src, image.Rect(128, 128, 256, 256)).(*image.RGBA)
	if p.Bounds() != image.Rect(0, 0, 128, 128) || p.Stride != src.Stride || &p.Pix[0] != &src.Pix[src.PixOffset(133, 135)] {
		t.Fatal("未使用正確的免複製 view")
	}
}
