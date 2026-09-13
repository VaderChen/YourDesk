//go:build darwin && cgo

package video

import (
	"image"
	"yourdesk/internal/optimization"
	"yourdesk/internal/softwarevideo"
)

type macAV1Decoder struct {
	decoder  *softwarevideo.Decoder
	software bool
	mode     string
}

func (d *macAV1Decoder) SetDecodePolicy(p optimization.Policy) {
	// 尺寸尚未由 AV1 sequence header 解析前，不外推其他尺寸的失敗證據。
}
func (d *macAV1Decoder) Decode(data []byte) (image.Image, error) {
	key, err := IsKeyframe(WireAV1, data)
	if err != nil {
		return nil, err
	}
	if d.decoder == nil && !d.software {
		d.decoder, err = softwarevideo.New(4)
		if err != nil {
			d.software = true
		}
	}
	if !d.software {
		im, e := d.decoder.Decode(data)
		if e == nil {
			d.mode = "hardware"
			return im, nil
		}
		d.decoder.Close()
		d.decoder = nil
		d.software = true
	}
	if d.decoder == nil {
		if !key {
			return nil, ErrNeedKeyframe
		}
		d.decoder, err = softwarevideo.New(3)
		if err != nil {
			return nil, err
		}
	}
	im, err := d.decoder.Decode(data)
	if err == nil {
		d.mode = "software"
	}
	return im, err
}
func (d *macAV1Decoder) Close() error {
	if d.decoder != nil {
		d.decoder.Close()
		d.decoder = nil
	}
	return nil
}
func (d *macAV1Decoder) Backend() string {
	if d.decoder != nil {
		return d.decoder.Backend()
	}
	return "AV1"
}
func (d *macAV1Decoder) DecodingMode() string { return d.mode }
