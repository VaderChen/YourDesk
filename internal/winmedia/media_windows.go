//go:build windows && cgo

// Package winmedia 將 Media Foundation／D3D11 限定在各自固定的 COM 執行緒。
package winmedia

/*
#cgo CXXFLAGS: -std=c++17 -D_WIN32_WINNT=0x0A00 -DWINVER=0x0A00
#cgo LDFLAGS: -static -static-libstdc++ -static-libgcc -lmfplat -lmf -lmfuuid -lmfreadwrite -ld3d11 -ldxgi -lole32 -loleaut32 -luuid -lstrmiids
#include "native.h"
*/
import "C"

import (
	"fmt"
	"image"
	"runtime"
	"sync"
	"unsafe"
)

type result struct {
	data, config []byte
	image        *image.RGBA
	backend      string
	err          error
}
type operation struct {
	run  func(*C.yd_media) result
	done chan result
}
type Session struct {
	mu       sync.Mutex
	queue    chan operation
	finished chan struct{}
	closed   bool
}

func New(mode int) (*Session, error) {
	s := &Session{queue: make(chan operation), finished: make(chan struct{})}
	ready := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(s.finished)
		var native *C.yd_media
		if hr := C.yd_media_open(C.int(mode), &native); hr < 0 {
			ready <- failure("建立硬體裝置", hr)
			return
		}
		defer C.yd_media_close(native)
		ready <- nil
		for op := range s.queue {
			op.done <- op.run(native)
		}
	}()
	if err := <-ready; err != nil {
		return nil, err
	}
	return s, nil
}
func failure(action string, hr C.int) error {
	return fmt.Errorf("Windows %s失敗（HRESULT 0x%08X）：%s", action, uint32(hr), C.GoString(C.yd_media_error()))
}
func (s *Session) call(run func(*C.yd_media) result) result {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return result{err: fmt.Errorf("硬體工作階段已關閉")}
	}
	op := operation{run: run, done: make(chan result, 1)}
	s.queue <- op
	return <-op.done
}
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	close(s.queue)
	<-s.finished
}
func validRGBA(src *image.RGBA) bool {
	return src != nil && !src.Bounds().Empty() && src.Stride >= src.Bounds().Dx()*4 && len(src.Pix) >= (src.Bounds().Dy()-1)*src.Stride+src.Bounds().Dx()*4
}
func (s *Session) Encode(src *image.RGBA, bitrate, fps, quality, gop int) ([]byte, []byte, string, error) {
	if !validRGBA(src) {
		return nil, nil, "", fmt.Errorf("無效的 RGBA 影格")
	}
	r := s.call(func(native *C.yd_media) result {
		var out C.yd_media_output
		defer C.yd_media_free(&out)
		hr := C.yd_media_encode(native, (*C.uchar)(unsafe.Pointer(&src.Pix[0])), C.int(src.Bounds().Dx()), C.int(src.Bounds().Dy()), C.int(src.Stride), C.int(bitrate), C.int(fps), C.int(quality), C.int(gop), &out)
		if hr < 0 {
			return result{err: failure("H.264 硬體編碼", hr)}
		}
		return result{data: C.GoBytes(unsafe.Pointer(out.data), C.int(out.size)), config: C.GoBytes(unsafe.Pointer(out.config), C.int(out.config_size)), backend: C.GoString(C.yd_media_backend(native))}
	})
	runtime.KeepAlive(src)
	return r.data, r.config, r.backend, r.err
}
func (s *Session) Decode(data []byte, width, height int) (*image.RGBA, string, error) {
	if len(data) == 0 || len(data) > 32<<20 {
		return nil, "", fmt.Errorf("無效的 H.264 影格")
	}
	r := s.call(func(native *C.yd_media) result {
		var out C.yd_media_output
		defer C.yd_media_free(&out)
		hr := C.yd_media_decode(native, (*C.uchar)(unsafe.Pointer(&data[0])), C.size_t(len(data)), C.int(width), C.int(height), &out)
		if hr < 0 {
			return result{err: failure("H.264 硬體解碼", hr)}
		}
		w, h := int(out.width), int(out.height)
		if w < 1 || h < 1 || w > 8192 || h > 8192 || w*h > 32<<20 || uint64(out.size) != uint64(w*h*4) {
			return result{err: fmt.Errorf("硬體解碼輸出尺寸無效")}
		}
		return result{image: &image.RGBA{Pix: C.GoBytes(unsafe.Pointer(out.data), C.int(out.size)), Stride: w * 4, Rect: image.Rect(0, 0, w, h)}, backend: C.GoString(C.yd_media_backend(native))}
	})
	return r.image, r.backend, r.err
}
func (s *Session) Scale(src, dst *image.RGBA) (string, error) {
	if !validRGBA(src) || !validRGBA(dst) {
		return "", fmt.Errorf("無效的縮圖緩衝")
	}
	r := s.call(func(native *C.yd_media) result {
		hr := C.yd_media_scale(native, (*C.uchar)(unsafe.Pointer(&src.Pix[0])), C.int(src.Bounds().Dx()), C.int(src.Bounds().Dy()), C.int(src.Stride), (*C.uchar)(unsafe.Pointer(&dst.Pix[0])), C.int(dst.Bounds().Dx()), C.int(dst.Bounds().Dy()), C.int(dst.Stride))
		if hr < 0 {
			return result{err: failure("D3D11 縮圖", hr)}
		}
		return result{backend: C.GoString(C.yd_media_backend(native))}
	})
	runtime.KeepAlive(src)
	runtime.KeepAlive(dst)
	return r.backend, r.err
}
