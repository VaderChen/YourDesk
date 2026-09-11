package input

import (
	"image"
	"math"
	"yourdesk/internal/rawkey"

	"github.com/kbinani/screenshot"
)

// Bounds 使用桌面邏輯座標，包含位於主螢幕左方或上方的負座標。
type Native struct{ Bounds image.Rectangle }

func (n Native) point(x, y float64) (float64, float64) {
	b := n.Bounds
	if b.Empty() {
		b = screenshot.GetDisplayBounds(0)
	}
	x = math.Max(0, math.Min(1, x))
	y = math.Max(0, math.Min(1, y))
	return float64(b.Min.X) + x*float64(max(0, b.Dx()-1)), float64(b.Min.Y) + y*float64(max(0, b.Dy()-1))
}

type Controller interface {
	RawKey(rawkey.Event) error
	Move(x, y float64) error
	Button(button int, down bool) error
	ButtonAt(button int, down bool, x, y float64) error
	Key(key string, down bool) error
	Wheel(delta float64) error
}
