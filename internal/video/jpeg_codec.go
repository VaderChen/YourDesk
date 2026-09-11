package video

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	"log/slog"
	"sync"
)

type JPEGEncoder interface {
	Encode(image.Image, int) ([]byte, error)
	Backend() string
	Hardware() bool
	Close() error
}

type JPEGDecoder interface {
	Decode([]byte) (image.Image, error)
	Backend() string
	Hardware() bool
	Close() error
}

type softwareJPEGEncoder struct{}

func (softwareJPEGEncoder) Encode(img image.Image, quality int) ([]byte, error) {
	var b bytes.Buffer
	if quality < 1 {
		quality = 1
	}
	if quality > 100 {
		quality = 100
	}
	err := jpeg.Encode(&b, img, &jpeg.Options{Quality: quality})
	return b.Bytes(), err
}
func (softwareJPEGEncoder) Backend() string { return "Go image/jpeg" }
func (softwareJPEGEncoder) Hardware() bool  { return false }
func (softwareJPEGEncoder) Close() error    { return nil }

type softwareJPEGDecoder struct{}

func (softwareJPEGDecoder) Decode(b []byte) (image.Image, error) {
	return jpeg.Decode(bytes.NewReader(b))
}
func (softwareJPEGDecoder) Backend() string { return "Go image/jpeg" }
func (softwareJPEGDecoder) Hardware() bool  { return false }
func (softwareJPEGDecoder) Close() error    { return nil }

type fallbackEncoder struct {
	mu       sync.Mutex
	current  JPEGEncoder
	fallback JPEGEncoder
	closed   bool
}

func (e *fallbackEncoder) Encode(img image.Image, q int) ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil, errors.New("JPEG encoder is closed")
	}
	out, err := e.current.Encode(img, q)
	if err == nil {
		return out, nil
	}
	if e.current.Hardware() {
		slog.Warn("hardware JPEG encode failed; falling back", "backend", e.current.Backend(), "error", err)
		_ = e.current.Close()
		e.current = e.fallback
		return e.current.Encode(img, q)
	}
	return nil, err
}
func (e *fallbackEncoder) Backend() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.current.Backend()
}
func (e *fallbackEncoder) Hardware() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.current.Hardware()
}
func (e *fallbackEncoder) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil
	}
	e.closed = true
	currentErr := e.current.Close()
	if e.fallback != nil && e.fallback != e.current {
		if fallbackErr := e.fallback.Close(); currentErr == nil {
			currentErr = fallbackErr
		}
	}
	return currentErr
}

type fallbackDecoder struct {
	mu       sync.Mutex
	current  JPEGDecoder
	fallback JPEGDecoder
	closed   bool
}

func (d *fallbackDecoder) Decode(b []byte) (image.Image, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, errors.New("JPEG decoder is closed")
	}
	img, err := d.current.Decode(b)
	if err == nil {
		return img, nil
	}
	if d.current.Hardware() {
		slog.Warn("hardware JPEG decode failed; falling back", "backend", d.current.Backend(), "error", err)
		_ = d.current.Close()
		d.current = d.fallback
		return d.current.Decode(b)
	}
	return nil, err
}
func (d *fallbackDecoder) Backend() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.current.Backend()
}
func (d *fallbackDecoder) Hardware() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.current.Hardware()
}
func (d *fallbackDecoder) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	currentErr := d.current.Close()
	if d.fallback != nil && d.fallback != d.current {
		if fallbackErr := d.fallback.Close(); currentErr == nil {
			currentErr = fallbackErr
		}
	}
	return currentErr
}

var ErrHardwareJPEGUnavailable = errors.New("hardware JPEG unavailable")

func NewJPEGEncoder(preferHardware bool) (JPEGEncoder, Selection) {
	sw := softwareJPEGEncoder{}
	if preferHardware {
		if hw, err := newHardwareJPEGEncoder(); err == nil {
			enc := &fallbackEncoder{current: hw, fallback: sw}
			return enc, Selection{Requested: CodecAuto, Selected: CodecHardwareJPEG, Backend: hw.Backend(), Hardware: true, Detail: "runtime encode probe passed"}
		}
	}
	return sw, Selection{Requested: CodecAuto, Selected: CodecSoftwareJPEG, Backend: sw.Backend(), Hardware: false, Detail: "hardware JPEG unavailable; software fallback"}
}

// NewJPEGEncoderForCodec 建立相容用 JPEG 編碼器。
// H.264／HEVC 由協商後的 IntraEncoder 處理；此路徑供舊 遠端顯示 與失敗回退使用。
func NewJPEGEncoderForCodec(requested Codec) (JPEGEncoder, Selection) {
	if requested == "" {
		requested = CodecAuto
	}
	if requested == CodecSoftwareJPEG {
		enc, selection := NewJPEGEncoder(false)
		selection.Requested = requested
		selection.Detail = "明確指定軟體 JPEG"
		return enc, selection
	}
	if requested == CodecHardwareJPEG || requested == CodecAuto {
		enc, selection := NewJPEGEncoder(true)
		selection.Requested = requested
		return enc, selection
	}
	// 影片編碼另行處理，保留可保證使用的軟體 JPEG 回退。
	enc, selection := NewJPEGEncoder(false)
	selection.Requested = requested
	selection.Detail = "影片協商前與失敗回退使用軟體 JPEG"
	return enc, selection
}

func NewJPEGDecoder(preferHardware bool) (JPEGDecoder, DecoderSelection) {
	sw := softwareJPEGDecoder{}
	if preferHardware {
		if hw, err := newHardwareJPEGDecoder(); err == nil {
			dec := &fallbackDecoder{current: hw, fallback: sw}
			return dec, DecoderSelection{Hardware: true, Backend: hw.Backend(), Codec: CodecHardwareJPEG, Probed: true, Detail: "runtime decode probe passed"}
		}
	}
	return sw, DecoderSelection{Hardware: false, Backend: sw.Backend(), Codec: CodecSoftwareJPEG, Probed: true, Detail: "hardware JPEG unavailable; software fallback"}
}

// NewJPEGDecoderForCodec mirrors NewJPEGEncoderForCodec.  A software request
// is strict; auto/hardware first attempts a native JPEG decoder and then
// permanently falls back to Go's decoder after the first runtime failure.
func NewJPEGDecoderForCodec(requested string) (JPEGDecoder, DecoderSelection) {
	if requested == "software" || requested == string(CodecSoftwareJPEG) {
		return NewJPEGDecoder(false)
	}
	return NewJPEGDecoder(true)
}
