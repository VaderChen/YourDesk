//go:build (!windows && !darwin) || !cgo

package video

import "image"

type platformDecoder struct {
	codec WireCodec
	mode  string
}

func newIntraDecoder(codec WireCodec) (IntraDecoder, error) {
	return &platformDecoder{codec: codec}, nil
}
func (d *platformDecoder) Decode(data []byte) (image.Image, error) {
	img, mode, err := decodeIntraStatus(d.codec, data)
	d.mode = mode
	return img, err
}
func (d *platformDecoder) Close() error { return nil }

func (d *platformDecoder) DecodingMode() string { return d.mode }
func (d *platformDecoder) Backend() string      { return "VideoToolbox / " + d.mode }
