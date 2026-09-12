package main

import (
	"bytes"
	"image"
	"image/draw"
)

// 解碼器回傳每張獨立的像素；完整緊密 RGBA 可直接移交合成畫面持有。
// 非完整更新仍沿用 draw.Src，完整更新不足畫面時亦保留原有清空行為。
func compositeDecodedFrame(previous *image.RGBA, decoded image.Image, bounds image.Rectangle, x, y int, keyframe bool) (*image.RGBA, bool) {
	src, rgba := decoded.(*image.RGBA)
	full := rgba && x == 0 && y == 0 && src.Bounds() == bounds && src.Stride == bounds.Dx()*4 && len(src.Pix) >= bounds.Dx()*bounds.Dy()*4
	if full {
		if previous != nil && previous.Bounds() == bounds && previous.Stride == src.Stride && bytes.Equal(previous.Pix, src.Pix) {
			return previous, false
		}
		return src, true
	}
	if previous == nil || previous.Bounds() != bounds || keyframe {
		previous = image.NewRGBA(bounds)
	}
	dst := image.Rect(x, y, x+decoded.Bounds().Dx(), y+decoded.Bounds().Dy())
	draw.Draw(previous, dst, decoded, decoded.Bounds().Min, draw.Src)
	return previous, true
}
