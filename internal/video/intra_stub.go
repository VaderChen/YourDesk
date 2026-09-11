//go:build (!darwin && !windows) || !cgo

package video

import "image"

func NewIntraEncoder(Codec) (IntraEncoder, error)        { return nil, ErrVideoUnavailable }
func DecodeIntra(WireCodec, []byte) (image.Image, error) { return nil, ErrVideoUnavailable }
