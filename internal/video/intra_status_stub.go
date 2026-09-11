//go:build !darwin || !cgo

package video

import "image"

func decodeIntraStatus(codec WireCodec, data []byte) (image.Image, string, error) {
	img, err := DecodeIntra(codec, data)
	return img, "unknown", err
}
