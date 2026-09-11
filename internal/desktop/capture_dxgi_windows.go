//go:build windows && cgo

package desktop

/*
#cgo CXXFLAGS: -std=c++17
#cgo LDFLAGS: -static -static-libstdc++ -static-libgcc -ld3d11 -ldxgi -luuid
#include "capture_dxgi_windows.h"
*/
import "C"
import (
	"fmt"
	"image"
	"log/slog"
	"time"
	"unsafe"
)

type dxgiCapture struct {
	native *C.yd_dxgi_capture
	bounds image.Rectangle
	retry  time.Time
}

func newDesktopGPU() desktopGPU { return &dxgiCapture{} }
func (c *dxgiCapture) close() {
	if c.native != nil {
		C.yd_dxgi_close(c.native)
		c.native = nil
	}
}
func (c *dxgiCapture) fail(hr C.int) error {
	c.close()
	c.retry = time.Now().Add(30 * time.Second)
	err := fmt.Errorf("DXGI 擷取不可用（HRESULT 0x%08X）", uint32(hr))
	slog.Warn("Windows 擷取退回 GDI，30 秒後重試", "原因", err)
	return err
}
func (c *dxgiCapture) capture(bounds image.Rectangle) (image.Image, error) {
	if bounds != c.bounds {
		c.close()
		c.bounds = bounds
		c.retry = time.Time{}
	}
	if c.native == nil {
		if time.Now().Before(c.retry) {
			return nil, fmt.Errorf("DXGI 等待重試")
		}
		hr := C.yd_dxgi_open(C.int(bounds.Min.X), C.int(bounds.Min.Y), C.int(bounds.Dx()), C.int(bounds.Dy()), &c.native)
		if hr < 0 {
			return nil, c.fail(hr)
		}
		slog.Info("Windows 硬體擷取啟用", "backend", "DXGI Desktop Duplication", "width", bounds.Dx(), "height", bounds.Dy())
	}
	out := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	hr := C.yd_dxgi_read(c.native, (*C.uchar)(unsafe.Pointer(&out.Pix[0])), C.int(bounds.Dx()), C.int(bounds.Dy()))
	if hr < 0 {
		return nil, c.fail(hr)
	}
	if hr == 1 {
		return nil, ErrNoNewFrame
	}
	return out, nil
}
