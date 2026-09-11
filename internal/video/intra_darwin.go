//go:build darwin && cgo

package video

/*
#cgo LDFLAGS: -framework VideoToolbox -framework CoreMedia -framework CoreVideo -framework CoreFoundation
#include "intra_darwin.h"
*/
import "C"

import (
	"fmt"
	"image"
	"image/draw"
	"sync"
	"unsafe"
)

type nativeIntraEncoder struct {
	mu                         sync.Mutex
	session                    C.VTCompressionSessionRef
	codec                      C.CMVideoCodecType
	width, height              int
	sequence                   int64
	closed                     bool
	bitrate, fps               int
	gop                        int
	appliedBitrate, appliedFPS int
}

func NewIntraEncoder(codec Codec) (IntraEncoder, error) {
	var kind C.CMVideoCodecType
	switch codec {
	case CodecHardwareH264:
		kind = C.kCMVideoCodecType_H264
	case CodecHardwareHEVC:
		kind = C.kCMVideoCodecType_HEVC
	default:
		return nil, ErrVideoUnavailable
	}
	return &nativeIntraEncoder{codec: kind, gop: 1}, nil
}
func (e *nativeIntraEncoder) SetKeyframeInterval(n int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	n = max(1, min(300, n))
	if e.gop != n {
		e.gop = n
		if e.session != 0 {
			C.yd_intra_close(e.session)
			e.session = 0
		}
	}
}
func (e *nativeIntraEncoder) SetRate(bitsPerSecond, fps int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.bitrate, e.fps = max(0, bitsPerSecond), max(1, fps)
}
func (e *nativeIntraEncoder) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.closed = true
	if e.session != 0 {
		C.yd_intra_close(e.session)
		e.session = 0
	}
	return nil
}
func (e *nativeIntraEncoder) Encode(src image.Image, quality int) ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed || src == nil {
		return nil, ErrVideoUnavailable
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 || w > 8192 || h > 8192 || int64(w)*int64(h) > 32*1024*1024 {
		return nil, fmt.Errorf("影像尺寸不支援")
	}
	// 4:2:0 硬體編碼採偶數尺寸，畫面顯示時依原始尺寸裁切。
	pw, ph := (w+1)&^1, (h+1)&^1
	if e.session == 0 || e.width != pw || e.height != ph {
		if e.session != 0 {
			C.yd_intra_close(e.session)
			e.session = 0
		}
		status := C.yd_intra_create(C.int(pw), C.int(ph), e.codec, C.int(e.gop), &e.session)
		if status != 0 {
			return nil, fmt.Errorf("VideoToolbox 建立硬體編碼器失敗：%d", status)
		}
		e.sequence = 0
		e.width, e.height = pw, ph
		e.appliedBitrate, e.appliedFPS = 0, 0
	}
	if e.bitrate > 0 && (e.bitrate != e.appliedBitrate || e.fps != e.appliedFPS) {
		if status := C.yd_intra_rate(e.session, C.int(e.bitrate), C.int(e.fps)); status != 0 {
			return nil, fmt.Errorf("VideoToolbox 設定碼率失敗：%d", status)
		}
		e.appliedBitrate, e.appliedFPS = e.bitrate, e.fps
	}
	fps := e.fps
	if fps < 1 {
		fps = 30
	}
	rgba := image.NewRGBA(image.Rect(0, 0, pw, ph))
	draw.Draw(rgba, image.Rect(0, 0, w, h), src, b.Min, draw.Src)
	var output *C.uchar
	var size C.size_t
	e.sequence++
	status := C.yd_intra_encode(e.session, (*C.uchar)(unsafe.Pointer(&rgba.Pix[0])), C.int(pw), C.int(ph), C.int(rgba.Stride), C.int64_t(e.sequence), C.int(quality), C.int(fps), C.int(e.gop), &output, &size)
	if output != nil {
		defer C.free(unsafe.Pointer(output))
	}
	if status != 0 || output == nil || size == 0 || uint64(size) > 32*1024*1024 {
		return nil, fmt.Errorf("VideoToolbox 硬體編碼失敗：%d", status)
	}
	return C.GoBytes(unsafe.Pointer(output), C.int(size)), nil
}
func DecodeIntra(codec WireCodec, payload []byte) (image.Image, error) {
	d, _ := newIntraDecoder(codec)
	defer d.Close()
	img, err := d.Decode(payload)
	return img, err
}

type platformDecoder struct {
	native C.yd_decoder
	codec  WireCodec
	mode   string
}

func newIntraDecoder(codec WireCodec) (IntraDecoder, error) {
	return &platformDecoder{codec: codec}, nil
}
func (d *platformDecoder) Close() error         { C.yd_decoder_close(&d.native); return nil }
func (d *platformDecoder) Backend() string      { return "VideoToolbox / " + d.mode }
func (d *platformDecoder) DecodingMode() string { return d.mode }
func (d *platformDecoder) Decode(payload []byte) (image.Image, error) {
	im, mode, err := d.decode(payload)
	d.mode = mode
	return im, err
}
func decodeIntraStatus(codec WireCodec, payload []byte) (image.Image, string, error) {
	d := &platformDecoder{codec: codec}
	defer d.Close()
	return d.decode(payload)
}
func (d *platformDecoder) decode(payload []byte) (image.Image, string, error) {
	codec := d.codec
	if len(payload) == 0 || len(payload) > 32*1024*1024 {
		return nil, "unknown", fmt.Errorf("無效壓縮影格")
	}
	var kind C.CMVideoCodecType
	switch codec {
	case WireH264:
		kind = C.kCMVideoCodecType_H264
	case WireHEVC:
		kind = C.kCMVideoCodecType_HEVC
	default:
		return nil, "unknown", ErrVideoUnavailable
	}
	var output *C.uchar
	var w, h, hardware C.int
	status := C.yd_intra_decode(&d.native, kind, (*C.uchar)(unsafe.Pointer(&payload[0])), C.size_t(len(payload)), &output, &w, &h, &hardware)
	if output != nil {
		defer C.free(unsafe.Pointer(output))
	}
	if status != 0 || output == nil {
		return nil, "unknown", fmt.Errorf("VideoToolbox 解碼失敗：%d", status)
	}
	mode := "unknown"
	if hardware == 1 {
		mode = "hardware"
	} else if hardware == 0 {
		mode = "software"
	}
	return &image.RGBA{Pix: C.GoBytes(unsafe.Pointer(output), C.int(w*h*4)), Stride: int(w) * 4, Rect: image.Rect(0, 0, int(w), int(h))}, mode, nil
}
