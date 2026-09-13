//go:build !cgo || !ffmpeg

package softwarevideo

import (
	"fmt"
	"image"
)

type Encoder struct{}

func NewAV1Encoder() (*Encoder, error) {
	return nil, fmt.Errorf("此建置未包含 FFmpeg AV1 編碼器")
}
func (*Encoder) Encode(*image.RGBA, int, int, int, int) ([]byte, error) {
	return nil, fmt.Errorf("FFmpeg AV1 編碼器未啟用")
}
func (*Encoder) Close() error { return nil }
