package desktop

import (
	"image"
	"log/slog"
	"math"
	"time"

	"golang.org/x/image/draw"
)

// StreamScaler 在編碼前等比例縮小，重用輸出緩衝；不放大來源。
type gpuScaler interface {
	Scale(*image.RGBA, *image.RGBA) error
	Close()
}
type StreamScaler struct {
	buffer  *image.RGBA
	gpu     gpuScaler
	retryAt time.Time
	warned  bool
}

func (s *StreamScaler) Close() {
	if s.gpu != nil {
		s.gpu.Close()
		s.gpu = nil
	}
	s.buffer = nil
}

func (s *StreamScaler) Reset() { s.buffer = nil }
func (s *StreamScaler) Scale(src image.Image, width, height int) image.Image {
	bounds := src.Bounds()
	if width < 1 || height < 1 || bounds.Empty() {
		return src
	}
	scale := math.Min(1, math.Min(float64(width)/float64(bounds.Dx()), float64(height)/float64(bounds.Dy())))
	if scale >= 1 {
		s.Reset()
		return src
	}
	w, h := max(1, int(float64(bounds.Dx())*scale)), max(1, int(float64(bounds.Dy())*scale))
	rect := image.Rect(0, 0, w, h)
	if s.buffer == nil || s.buffer.Bounds() != rect {
		s.buffer = image.NewRGBA(rect)
	}
	if rgba, ok := src.(*image.RGBA); ok && len(rgba.Pix) >= rgba.Stride*rgba.Bounds().Dy() {
		if s.gpu == nil && time.Now().After(s.retryAt) {
			var err error
			s.gpu, err = newGPUScaler()
			if err != nil {
				s.fallback(err)
			} else {
				s.warned = false
				slog.Info("串流縮圖使用 GPU")
			}
		}
		if s.gpu != nil {
			if err := s.gpu.Scale(rgba, s.buffer); err == nil {
				return s.buffer
			} else {
				s.gpu.Close()
				s.gpu = nil
				s.fallback(err)
			}
		}
	}
	draw.ApproxBiLinear.Scale(s.buffer, rect, src, bounds, draw.Src, nil)
	return s.buffer
}

func (s *StreamScaler) fallback(err error) {
	s.retryAt = time.Now().Add(30 * time.Second)
	if !s.warned {
		slog.Warn("GPU 縮圖不可用，改用 CPU 備援", "error", err)
		s.warned = true
	}
}
