//go:build windows

package desktop

import (
	"errors"
	"fmt"
	"image"
	"runtime"
	"sync"
	"unsafe"

	"github.com/lxn/win"
)

type captureReply struct {
	image image.Image
	err   error
}
type captureRequest struct {
	display int
	reply   chan captureReply
}

type desktopGPU interface {
	capture(image.Rectangle) (image.Image, error)
	close()
}

// 持續工作執行緒持有 DXGI／GDI 資源，優先使用硬體擷取。
type windowsCapturer struct {
	ScreenshotCapturer
	mu       sync.Mutex
	requests chan captureRequest
	done     chan struct{}
	closed   bool
}

func NewLiveCapturer() LiveCapturer {
	c := &windowsCapturer{requests: make(chan captureRequest), done: make(chan struct{})}
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(c.done)
		var surface gdiSurface
		defer surface.close()
		gpu := newDesktopGPU()
		if gpu != nil {
			defer gpu.close()
		}
		for request := range c.requests {
			bounds := c.Bounds(request.display)
			if gpu != nil && !bounds.Empty() {
				img, err := gpu.capture(bounds)
				if err == nil || errors.Is(err, ErrNoNewFrame) {
					surface.close()
					request.reply <- captureReply{img, err}
					continue
				}
			}
			img, err := surface.capture(bounds)
			if err != nil {
				surface.close()
			}
			request.reply <- captureReply{img, err}
		}
	}()
	return c
}
func (c *windowsCapturer) Capture(display int) (image.Image, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, fmt.Errorf("擷取工作階段已關閉")
	}
	reply := make(chan captureReply, 1)
	c.requests <- captureRequest{display, reply}
	result := <-reply
	return result.image, result.err
}
func (c *windowsCapturer) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	close(c.requests)
	<-c.done
}

// 單次截圖亦使用相同 DIB 實作，不再走套件的 GetDIBits 路徑。
func captureDisplay(display int) (image.Image, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var surface gdiSurface
	defer surface.close()
	return surface.capture((ScreenshotCapturer{}).Bounds(display))
}

type gdiSurface struct {
	bounds         image.Rectangle
	screen, memory win.HDC
	bitmap         win.HBITMAP
	previous       win.HGDIOBJ
	bits           unsafe.Pointer
}

func (s *gdiSurface) close() {
	if s.memory != 0 && s.previous != 0 {
		win.SelectObject(s.memory, s.previous)
	}
	if s.bitmap != 0 {
		win.DeleteObject(win.HGDIOBJ(s.bitmap))
	}
	if s.memory != 0 {
		win.DeleteDC(s.memory)
	}
	if s.screen != 0 {
		win.ReleaseDC(0, s.screen)
	}
	*s = gdiSurface{}
}
func (s *gdiSurface) prepare(bounds image.Rectangle) error {
	if bounds.Empty() || bounds.Dx() > 8192 || bounds.Dy() > 8192 || int64(bounds.Dx())*int64(bounds.Dy()) > 32<<20 {
		return fmt.Errorf("擷取螢幕尺寸無效：%v", bounds)
	}
	if s.bitmap != 0 && s.bounds == bounds {
		return nil
	}
	s.close()
	s.screen = win.GetDC(0)
	if s.screen == 0 {
		return fmt.Errorf("GetDC 失敗")
	}
	s.memory = win.CreateCompatibleDC(s.screen)
	if s.memory == 0 {
		return fmt.Errorf("CreateCompatibleDC 失敗")
	}
	header := win.BITMAPINFOHEADER{BiSize: uint32(unsafe.Sizeof(win.BITMAPINFOHEADER{})), BiWidth: int32(bounds.Dx()), BiHeight: -int32(bounds.Dy()), BiPlanes: 1, BiBitCount: 32, BiCompression: win.BI_RGB}
	s.bitmap = win.CreateDIBSection(s.screen, &header, win.DIB_RGB_COLORS, &s.bits, 0, 0)
	if s.bitmap == 0 || s.bits == nil {
		return fmt.Errorf("CreateDIBSection 失敗")
	}
	previous := win.SelectObject(s.memory, win.HGDIOBJ(s.bitmap))
	if previous == 0 || uintptr(previous) == ^uintptr(0) {
		return fmt.Errorf("SelectObject 失敗")
	}
	s.previous = previous
	s.bounds = bounds
	return nil
}
func (s *gdiSurface) capture(bounds image.Rectangle) (image.Image, error) {
	if err := s.prepare(bounds); err != nil {
		return nil, err
	}
	w, h := bounds.Dx(), bounds.Dy()
	if !win.BitBlt(s.memory, 0, 0, int32(w), int32(h), s.screen, int32(bounds.Min.X), int32(bounds.Min.Y), win.SRCCOPY) {
		return nil, fmt.Errorf("BitBlt 失敗")
	}
	// 直接讀 DIB 前，須先讓 GDI 完成批次繪製。
	if !win.GdiFlush() {
		return nil, fmt.Errorf("GdiFlush 失敗")
	}
	src := unsafe.Slice((*byte)(s.bits), w*h*4)
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	// 每幀輸出獨立記憶體，避免下一次 BitBlt 覆寫編碼中的畫面。
	for i := 0; i < len(src); i += 4 {
		out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = src[i+2], src[i+1], src[i], 255
	}
	return out, nil
}
