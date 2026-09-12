//go:build !cgo || !ffmpeg

package softwarevideo

import (
	"errors"
	"image"
)

type Decoder struct{}

func New(int) (*Decoder, error) {
	return nil, errors.New("此建置未包含 FFmpeg；Windows 發行版需使用 ffmpeg build tag")
}
func (*Decoder) Decode([]byte) (*image.RGBA, error) { return nil, errors.New("FFmpeg 未啟用") }
func (*Decoder) Close() error                       { return nil }
func (*Decoder) Backend() string                    { return "FFmpeg 未啟用" }
func (*Decoder) DecodingMode() string               { return "unknown" }
