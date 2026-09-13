package video

import (
	"image"
	"image/draw"
	"sync"
	"yourdesk/internal/softwarevideo"
)

type softwareAV1Encoder struct {
	mu             sync.Mutex
	encoder        *softwarevideo.Encoder
	sequence       []byte
	rate, fps, gop int
	closed         bool
}

func newSoftwareAV1Encoder() (IntraEncoder, error) {
	e, err := softwarevideo.NewAV1Encoder()
	if err != nil {
		return nil, err
	}
	return &softwareAV1Encoder{encoder: e, fps: 30, gop: 1}, nil
}
func (e *softwareAV1Encoder) Backend() string { return "FFmpeg / libaom AV1 CPU" }
func (e *softwareAV1Encoder) SetRate(rate, fps int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rate = max(0, rate)
	e.fps = max(1, min(240, fps))
}
func (e *softwareAV1Encoder) SetKeyframeInterval(gop int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.gop = max(1, min(300, gop))
}
func (e *softwareAV1Encoder) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.closed = true
	return e.encoder.Close()
}
func (e *softwareAV1Encoder) Encode(src image.Image, quality int) ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed || src == nil {
		return nil, ErrVideoUnavailable
	}
	b := src.Bounds()
	w, h := (b.Dx()+1)&^1, (b.Dy()+1)&^1
	if b.Dx() <= 0 || b.Dy() <= 0 || w > 8192 || h > 8192 || w*h > 32<<20 {
		return nil, ErrVideoUnavailable
	}
	pixels := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(pixels, image.Rect(0, 0, b.Dx(), b.Dy()), src, b.Min, draw.Src)
	if w > b.Dx() {
		for y := 0; y < b.Dy(); y++ {
			copy(pixels.Pix[y*pixels.Stride+(w-1)*4:], pixels.Pix[y*pixels.Stride+(w-2)*4:y*pixels.Stride+(w-1)*4])
		}
	}
	if h > b.Dy() {
		copy(pixels.Pix[(h-1)*pixels.Stride:], pixels.Pix[(h-2)*pixels.Stride:(h-1)*pixels.Stride])
	}
	data, err := e.encoder.Encode(pixels, e.rate, e.fps, e.gop, quality)
	if err != nil {
		return nil, err
	}
	return packAV1(data, nil, &e.sequence)
}
