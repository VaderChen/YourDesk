//go:build cgo && turbojpeg

package video

/*
#cgo LDFLAGS: -lturbojpeg
#include <turbojpeg.h>
// C 擁有輸出配置；指標的指標不存放 Go 指標。
typedef struct { unsigned char *data; unsigned long size; int status; } yd_turbo_result;
static yd_turbo_result yd_turbo_encode(tjhandle h,unsigned char *p,int w,int stride,int height,int quality) {
 yd_turbo_result r={0};r.status=tjCompress2(h,p,w,stride,height,TJPF_RGBA,&r.data,&r.size,TJSAMP_420,quality,TJFLAG_ACCURATEDCT);return r;
}
*/
import "C"
import (
	"errors"
	"image"
	"image/draw"
	"sync"
	"unsafe"
)

const preferredSoftwareJPEGBackend = "TurboJPEG CPU SIMD"

type turboJPEGEncoder struct {
	mu     sync.Mutex
	handle C.tjhandle
	pixels *image.RGBA
}
type turboJPEGDecoder struct {
	mu     sync.Mutex
	handle C.tjhandle
}

func newTurboJPEGEncoder() JPEGEncoder {
	h := C.tjInitCompress()
	if h == nil {
		return nil
	}
	return &turboJPEGEncoder{handle: h}
}
func newTurboJPEGDecoder() JPEGDecoder {
	h := C.tjInitDecompress()
	if h == nil {
		return nil
	}
	return &turboJPEGDecoder{handle: h}
}
func (*turboJPEGEncoder) Backend() string { return "TurboJPEG CPU SIMD" }
func (*turboJPEGDecoder) Backend() string { return "TurboJPEG CPU SIMD" }
func (*turboJPEGEncoder) Hardware() bool  { return false }
func (*turboJPEGDecoder) Hardware() bool  { return false }
func (e *turboJPEGEncoder) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.handle != nil {
		C.tjDestroy(e.handle)
		e.handle = nil
	}
	e.pixels = nil
	return nil
}
func (d *turboJPEGDecoder) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.handle != nil {
		C.tjDestroy(d.handle)
		d.handle = nil
	}
	return nil
}
func (e *turboJPEGEncoder) Encode(src image.Image, quality int) ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.handle == nil || src == nil {
		return nil, errors.New("TurboJPEG 編碼器未就緒")
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 1 || h < 1 || w > 8192 || h > 8192 || int64(w)*int64(h) > 32<<20 {
		return nil, errors.New("TurboJPEG 影像尺寸不支援")
	}
	rgba, ok := src.(*image.RGBA)
	if !ok {
		if e.pixels == nil || e.pixels.Bounds().Size() != b.Size() {
			e.pixels = image.NewRGBA(image.Rect(0, 0, w, h))
		}
		rgba = e.pixels
		draw.Draw(rgba, rgba.Bounds(), src, b.Min, draw.Src)
	}
	if rgba.Stride < w*4 || len(rgba.Pix) < (h-1)*rgba.Stride+w*4 {
		return nil, errors.New("TurboJPEG 像素緩衝區不足")
	}
	r := C.yd_turbo_encode(e.handle, (*C.uchar)(unsafe.Pointer(&rgba.Pix[0])), C.int(w), C.int(rgba.Stride), C.int(h), C.int(max(1, min(100, quality))))
	if r.data != nil {
		defer C.tjFree(r.data)
	}
	if r.status != 0 || r.data == nil || r.size == 0 || uint64(r.size) > 64<<20 {
		return nil, errors.New("TurboJPEG 編碼失敗")
	}
	return C.GoBytes(unsafe.Pointer(r.data), C.int(r.size)), nil
}
func (d *turboJPEGDecoder) Decode(data []byte) (image.Image, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.handle == nil || len(data) == 0 || len(data) > 64<<20 {
		return nil, errors.New("TurboJPEG 輸入無效")
	}
	var w, h, sampling, space C.int
	p := (*C.uchar)(unsafe.Pointer(&data[0]))
	if C.tjDecompressHeader3(d.handle, p, C.ulong(len(data)), &w, &h, &sampling, &space) != 0 || w < 1 || h < 1 || w > 8192 || h > 8192 || int64(w)*int64(h) > 32<<20 {
		return nil, errors.New("TurboJPEG 標頭或尺寸無效")
	}
	// 每張輸出獨立配置，避免下一次解碼覆寫顯示端仍持有的像素。
	dst := image.NewRGBA(image.Rect(0, 0, int(w), int(h)))
	if C.tjDecompress2(d.handle, p, C.ulong(len(data)), (*C.uchar)(unsafe.Pointer(&dst.Pix[0])), w, C.int(dst.Stride), h, C.TJPF_RGBA, C.TJFLAG_ACCURATEDCT) != 0 {
		return nil, errors.New("TurboJPEG 解碼失敗")
	}
	return dst, nil
}
