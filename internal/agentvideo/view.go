// Package agentvideo 定義串流與按需截圖共用的視野及座標。
package agentvideo

import (
	"fmt"
	"image"
	"image/draw"
	"math"
)

type Region struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

func Full() Region { return Region{Width: 1, Height: 1} }
func (r Region) Validate() error {
	for _, v := range []float64{r.X, r.Y, r.Width, r.Height} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("區域必須為有限數值")
		}
	}
	if r.X < 0 || r.Y < 0 || r.Width <= 0 || r.Height <= 0 || r.X+r.Width > 1 || r.Y+r.Height > 1 {
		return fmt.Errorf("區域使用螢幕相對座標 0～1，寬高須大於零且不得超出螢幕")
	}
	return nil
}

// Bounds 在影像像素與桌面邏輯座標分別運算，保留 Retina 比例及負原點。
func (r Region) Bounds(b image.Rectangle) image.Rectangle {
	if b.Empty() {
		return image.Rectangle{}
	}
	x := min(b.Max.X-1, b.Min.X+int(math.Floor(r.X*float64(b.Dx()))))
	y := min(b.Max.Y-1, b.Min.Y+int(math.Floor(r.Y*float64(b.Dy()))))
	return image.Rect(x, y, min(b.Max.X, max(x+1, b.Min.X+int(math.Ceil((r.X+r.Width)*float64(b.Dx()))))), min(b.Max.Y, max(y+1, b.Min.Y+int(math.Ceil((r.Y+r.Height)*float64(b.Dy()))))))
}
func (r Region) Crop(src image.Image) *image.RGBA {
	b := r.Bounds(src.Bounds())
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), src, b.Min, draw.Src)
	return out
}

type Request struct {
	Mode       string  `json:"mode,omitempty"`
	Region     *Region `json:"region,omitempty"`
	FullScreen bool    `json:"fullScreen,omitempty"`
}

func (r Request) Validate() error {
	if r.Mode != "" && r.Mode != "paused" && r.Mode != "streaming" {
		return fmt.Errorf("mode 須為 paused 或 streaming")
	}
	if r.Region != nil {
		if r.FullScreen {
			return fmt.Errorf("region 與 fullScreen 不可同時指定")
		}
		return r.Region.Validate()
	}
	return nil
}

type State struct {
	ViewID       uint64 `json:"viewID"`
	Mode         string `json:"mode"`
	Region       Region `json:"region"`
	Display      int    `json:"display"`
	DisplayCount int    `json:"displayCount"`
	Width        int    `json:"width,omitempty"`
	Height       int    `json:"height,omitempty"`
	Token        string `json:"token,omitempty"`
	Bytes        int    `json:"bytes,omitempty"`
}
type Read struct {
	Token  string `json:"token"`
	Offset int    `json:"offset"`
}

const ChunkSize = 8192
const MaxBytes = 8 * 1024 * 1024
