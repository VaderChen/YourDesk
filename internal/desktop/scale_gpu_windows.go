//go:build windows && cgo

package desktop

import (
	"image"
	"log/slog"
	"yourdesk/internal/winmedia"
)

type windowsGPUScaler struct {
	session *winmedia.Session
	logged  bool
}

func newGPUScaler() (gpuScaler, error) {
	session, err := winmedia.New(0)
	if err != nil {
		return nil, err
	}
	return &windowsGPUScaler{session: session}, nil
}
func (s *windowsGPUScaler) Scale(src, dst *image.RGBA) error {
	backend, err := s.session.Scale(src, dst)
	if err == nil && !s.logged {
		s.logged = true
		slog.Info("Windows GPU 縮圖後端", "backend", backend)
	}
	return err
}
func (s *windowsGPUScaler) Close() { s.session.Close() }
