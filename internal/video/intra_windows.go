//go:build windows && cgo

package video

import (
	"image"
	"image/draw"
	"sync"
	"yourdesk/internal/optimization"
	"yourdesk/internal/softwarevideo"
	"yourdesk/internal/winmedia"
)

type windowsEncoder struct {
	codec        WireCodec
	sequence     []byte
	mu           sync.Mutex
	session      *winmedia.Session
	pixels       *image.RGBA
	bitrate, fps int
	gop          int
	backend      string
	closed       bool
}

func NewIntraEncoder(codec Codec) (IntraEncoder, error) {
	if codec != CodecHardwareH264 && codec != CodecHardwareAV1 {
		return nil, ErrVideoUnavailable
	}
	session, err := winmedia.New(1)
	if err != nil {
		return nil, err
	}
	return &windowsEncoder{codec: WireForCodec(codec), session: session, fps: 30, gop: 1}, nil
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
	data, config, backend, err := e.session.EncodeCodec(e.pixels, rate, e.fps, max(1, min(100, quality)), e.gop, int(e.codec))
	if err != nil {
		return nil, err
	}
	e.backend = backend
	if e.codec == WireAV1 {
		return packAV1(data, config, &e.sequence)
	}
	return packH264(data, config)
}

// hardwareOnly 的 MF session 失敗後由 FFmpeg 接手，避免依賴系統 codec extension。
type windowsDecoder struct {
	session  *winmedia.Session
	cpu      *softwarevideo.Decoder
	mu       sync.Mutex
	backend  string
	policy   optimization.Policy
	codec    WireCodec
	mode     string
	software bool
}

func (d *windowsDecoder) SetDecodePolicy(p optimization.Policy) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.policy = p
}
func newIntraDecoder(codec WireCodec) (IntraDecoder, error) {
	if codec != WireH264 && codec != WireHEVC && codec != WireAV1 {
		return nil, ErrVideoUnavailable
	}
	return &windowsDecoder{codec: codec, mode: "unknown"}, nil
}
func (d *windowsDecoder) Decode(payload []byte) (image.Image, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	key, err := IsKeyframe(d.codec, payload)
	if err != nil {
		return nil, err
	}
	var data []byte
	width, height := 0, 0
	switch d.codec {
	case WireH264:
		data, err = unpackH264(payload)
		if err == nil {
			width, height, err = h264Dimensions(data)
		}
	case WireHEVC:
		data, err = unpackHEVC(payload)
	case WireAV1:
		data = payload
	}
	if err != nil {
		return nil, err
	}
	if d.policy.DecoderDecision(decodePolicyCodec(d.codec), width, height) == "software" {
		d.software = true
	}
	if !d.software {
		if d.session == nil {
			d.session, err = winmedia.New(4)
		}
		if err == nil {
			pixels, backend, _, e := d.session.DecodeCodec(data, width, height, int(d.codec))
			if e == nil {
				d.backend = backend
				d.mode = "hardware"
				return pixels, nil
			}
		}
		d.software = true
		if d.session != nil {
			d.session.Close()
			d.session = nil
		}
	}
	if d.cpu == nil {
		// 中途切換解碼器沒有先前參考影格，保留退回狀態並要求下一張 key frame。
		if !key {
			return nil, ErrNeedKeyframe
		}
		d.cpu, err = softwarevideo.New(int(d.codec))
		if err != nil {
			return nil, err
		}
	}
	pixels, err := d.cpu.Decode(data)
	if err == nil {
		d.backend = d.cpu.Backend()
		d.mode = "software"
	}
	return pixels, err
}
func (d *windowsDecoder) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.session != nil {
		d.session.Close()
		d.session = nil
	}
	if d.cpu != nil {
		d.cpu.Close()
		d.cpu = nil
	}
	return nil
}
func (d *windowsDecoder) Backend() string { d.mu.Lock(); defer d.mu.Unlock(); return d.backend }
func DecodeIntra(codec WireCodec, payload []byte) (image.Image, error) {
	decoder, err := newIntraDecoder(codec)
	if err != nil {
		return nil, err
	}
	defer decoder.Close()
	return decoder.Decode(payload)
}

func (d *windowsDecoder) DecodingMode() string { d.mu.Lock(); defer d.mu.Unlock(); return d.mode }
