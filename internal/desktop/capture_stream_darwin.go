//go:build darwin && cgo

package desktop

/*
#cgo LDFLAGS: -framework Foundation -framework ScreenCaptureKit -framework CoreMedia -framework CoreVideo -framework CoreGraphics
#include <stdlib.h>
void *yd_capture_open(int,int,int,char*,int);
int yd_capture_read(void*,unsigned char*,int,int,char*,int);
void yd_capture_close(void*);
unsigned int yd_capture_display_id(int);
*/
import "C"
import (
	"fmt"
	"image"
	"log/slog"
	"time"
	"unsafe"
)

type liveCapturer struct {
	ScreenshotCapturer
	stream unsafe.Pointer
	id     uint32
	w, h   int
	retry  time.Time
}

func NewLiveCapturer() LiveCapturer { return &liveCapturer{} }
func (c *liveCapturer) Close() {
	if c.stream != nil {
		C.yd_capture_close(c.stream)
		c.stream = nil
	}
}
func (c *liveCapturer) Capture(display int) (image.Image, error) {
	b := c.Bounds(display)
	if b.Empty() {
		return nil, fmt.Errorf("擷取螢幕不存在")
	}
	id := uint32(C.yd_capture_display_id(C.int(display)))
	if c.id != id || c.w != b.Dx() || c.h != b.Dy() {
		c.Close()
		c.id = id
		c.w = b.Dx()
		c.h = b.Dy()
		c.retry = time.Time{}
	}
	if c.stream == nil && time.Now().After(c.retry) {
		var message [512]C.char
		c.stream = C.yd_capture_open(C.int(display), C.int(c.w), C.int(c.h), &message[0], 512)
		if c.stream == nil {
			c.retry = time.Now().Add(30 * time.Second)
			slog.Warn("持續擷取不可用，暫用截圖", "原因", C.GoString(&message[0]))
		} else {
			slog.Info("持續擷取啟用", "width", c.w, "height", c.h)
		}
	}
	if c.stream == nil {
		return c.ScreenshotCapturer.Capture(display)
	}
	out := image.NewRGBA(image.Rect(0, 0, c.w, c.h))
	var message [512]C.char
	status := int(C.yd_capture_read(c.stream, (*C.uchar)(unsafe.Pointer(&out.Pix[0])), C.int(c.w), C.int(c.h), &message[0], 512))
	if status == 1 {
		return out, nil
	}
	if status == 0 {
		return nil, ErrNoNewFrame
	}
	c.Close()
	c.retry = time.Now().Add(30 * time.Second)
	slog.Warn("持續擷取中斷，暫用截圖", "原因", C.GoString(&message[0]))
	return c.ScreenshotCapturer.Capture(display)
}
