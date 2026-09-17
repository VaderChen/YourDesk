package main

import (
	"github.com/hajimehoshi/ebiten/v2"
	"math"
)

// 繪圖、工具列與裁切共用最終畫布座標；原生樣本尚未就緒時沿用引擎。
func (g *game) pointerPosition() (float64, float64) {
	x, y := ebiten.CursorPosition()
	px, py := g.finalTransform.Apply(float64(x), float64(y))
	nx, ny, valid := nativePointer()
	return canvasPointer(px, py, nx, ny, g.finalWidth, g.finalHeight, valid)
}

func canvasPointer(px, py, nx, ny float64, width, height int, valid bool) (float64, float64) {
	if !valid || width <= 0 || height <= 0 || math.IsNaN(nx) || math.IsNaN(ny) || math.IsInf(nx, 0) || math.IsInf(ny, 0) {
		return px, py
	}
	// 不限制到 [0,1]；視窗外座標必須仍在外面，讓命中檢查拒絕輸入。
	return nx * float64(width), ny * float64(height)
}
