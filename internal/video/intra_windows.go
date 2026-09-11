//go:build windows && cgo

package video

import (
	"image"
	"image/draw"
	"sync"
	"yourdesk/internal/winmedia"
)

type windowsEncoder struct {
	mu           sync.Mutex
	session      *winmedia.Session
	pixels       *image.RGBA
	bitrate, fps int
	gop          int
	backend      string
	closed       bool
}

func NewIntraEncoder(codec Codec) (IntraEncoder, error) {
	if codec != CodecHardwareH264 {
		return nil, ErrVideoUnavailable
	}
	session, err := winmedia.New(1)
	if err != nil {
		return nil, err
	}
	return &windowsEncoder{session: session, fps: 30, gop: 1}, nil
}
func (e *windowsEncoder) SetKeyframeInterval(n int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.gop = max(1, min(300, n))
}
func (e *windowsEncoder) SetRate(rate, fps int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.bitrate = rate
	e.fps = min(240, max(1, fps))
}
func (e *windowsEncoder) Backend() string { e.mu.Lock(); defer e.mu.Unlock(); return e.backend }
func (e *windowsEncoder) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.closed {
		e.closed = true
		e.session.Close()
		e.pixels = nil
	}
	return nil
}
func (e *windowsEncoder) Encode(src image.Image, quality int) ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed || src == nil {
		return nil, ErrVideoUnavailable
	}
	b := src.Bounds()
	w, h := (b.Dx()+1)&^1, (b.Dy()+1)&^1
	if w < 1 || h < 1 || w > 8192 || h > 8192 || w*h > 32<<20 {
		return nil, ErrVideoUnavailable
	}
	rect := image.Rect(0, 0, w, h)
	if e.pixels == nil || e.pixels.Bounds() != rect {
		e.pixels = image.NewRGBA(rect)
	}
	draw.Draw(e.pixels, image.Rect(0, 0, b.Dx(), b.Dy()), src, b.Min, draw.Src)
	// 延伸邊緣填滿 4:2:0 的偶數尺寸，避免黑色邊緣污染最後一列／行。
	if w > b.Dx() {
		for y := 0; y < b.Dy(); y++ {
			copy(e.pixels.Pix[y*e.pixels.Stride+(w-1)*4:], e.pixels.Pix[y*e.pixels.Stride+(w-2)*4:y*e.pixels.Stride+(w-1)*4])
		}
	}
	if h > b.Dy() {
		copy(e.pixels.Pix[(h-1)*e.pixels.Stride:], e.pixels.Pix[(h-2)*e.pixels.Stride:(h-1)*e.pixels.Stride])
	}
	// 與 Mac 一致：零碼率使用品質控制，僅快速模式指定碼率。
	rate := max(0, e.bitrate)
	data, config, backend, err := e.session.Encode(e.pixels, rate, e.fps, max(1, min(100, quality)), e.gop)
	if err != nil {
		return nil, err
	}
	e.backend = backend
	return packH264(data, config)
}

type windowsDecoder struct {
	session *winmedia.Session
	mu      sync.Mutex
	backend string
}

func newIntraDecoder(codec WireCodec) (IntraDecoder, error) {
	if codec != WireH264 {
		return nil, ErrVideoUnavailable
	}
	session, err := winmedia.New(2)
	if err != nil {
		return nil, err
	}
	return &windowsDecoder{session: session}, nil
}
func (d *windowsDecoder) Decode(payload []byte) (image.Image, error) {
	data, err := unpackH264(payload)
	if err != nil {
		return nil, err
	}
	width, height, err := h264Dimensions(data)
	if err != nil {
		return nil, err
	}
	pixels, backend, err := d.session.Decode(data, width, height)
	d.mu.Lock()
	d.backend = backend
	d.mu.Unlock()
	return pixels, err
}
func (d *windowsDecoder) Close() error    { d.session.Close(); return nil }
func (d *windowsDecoder) Backend() string { d.mu.Lock(); defer d.mu.Unlock(); return d.backend }
func DecodeIntra(codec WireCodec, payload []byte) (image.Image, error) {
	decoder, err := newIntraDecoder(codec)
	if err != nil {
		return nil, err
	}
	defer decoder.Close()
	return decoder.Decode(payload)
}

func (d *windowsDecoder) DecodingMode() string { return "hardware" }
