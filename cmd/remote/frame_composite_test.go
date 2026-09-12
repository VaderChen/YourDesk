package main

import (
	"image"
	"image/color"
	"testing"
)

func TestCompositeFrameOwnershipAndIdle(t *testing.T) {
	b := image.Rect(0, 0, 4, 3)
	src := image.NewRGBA(b)
	src.SetRGBA(1, 1, color.RGBA{20, 30, 40, 255})
	out, changed := compositeDecodedFrame(nil, src, b, 0, 0, true)
	if out != src || !changed {
		t.Fatal("完整 RGBA 未移交所有權")
	}
	duplicate := image.NewRGBA(b)
	copy(duplicate.Pix, src.Pix)
	out, changed = compositeDecodedFrame(out, duplicate, b, 0, 0, true)
	if out != src || changed {
		t.Fatal("相同完整影格不應重畫")
	}
	patch := image.NewRGBA(image.Rect(0, 0, 1, 1))
	patch.SetRGBA(0, 0, color.RGBA{80, 70, 60, 255})
	out, changed = compositeDecodedFrame(out, patch, b, 2, 1, false)
	if !changed || out.RGBAAt(2, 1) != patch.RGBAAt(0, 0) || out.RGBAAt(1, 1) != src.RGBAAt(1, 1) {
		t.Fatal("區塊合成結果不符")
	}
}

func TestCompositeFrameStrideAndKeyframeClear(t *testing.T) {
	b := image.Rect(0, 0, 3, 2)
	parent := image.NewRGBA(image.Rect(0, 0, 8, 4))
	src := parent.SubImage(image.Rect(1, 1, 4, 3)).(*image.RGBA)
	src.SetRGBA(1, 1, color.RGBA{1, 2, 3, 255})
	out, _ := compositeDecodedFrame(nil, src, b, 0, 0, true)
	if out == src || out.RGBAAt(0, 0) != (color.RGBA{1, 2, 3, 255}) {
		t.Fatal("非零起點或 stride 處理不符")
	}
	patch := image.NewRGBA(image.Rect(0, 0, 1, 1))
	out, _ = compositeDecodedFrame(out, patch, b, 2, 1, true)
	if out.RGBAAt(0, 0) != (color.RGBA{}) {
		t.Fatal("不完整 keyframe 未清空其餘畫面")
	}
}
