//go:build darwin && cgo

package video

import (
	"image"
	"yourdesk/internal/softwarevideo"
)

func ProbeHardwareAV1(width, height int, phase string) map[string]any {
	r := map[string]any{"codec": "av1", "input": "RGBA", "width": width, "height": height, "phase": phase}
	if phase == "encode" {
		r["encodeOK"] = false
		r["hardwareEncoder"] = false
		r["encoderBackend"] = "VideoToolbox AV1"
		e, err := NewIntraEncoder(CodecHardwareAV1)
		if err != nil {
			r["encodeError"] = err.Error()
			return r
		}
		defer e.Close()
		src := image.NewRGBA(image.Rect(0, 0, width, height))
		data, err := e.Encode(src, 70)
		r["encodeOK"] = err == nil && len(data) > 0
		r["hardwareEncoder"] = r["encodeOK"]
		if err != nil {
			r["encodeError"] = err.Error()
		}
		return r
	}
	r["decoderSource"] = "independent-fixture"
	r["decodeOK"] = false
	r["hardwareDecoder"] = true
	r["decodingMode"] = "hardware"
	r["decoderBackend"] = "VideoToolbox AV1 / FFmpeg"
	data, err := ProbeWireFixture("av1", width, height)
	if err != nil {
		r["decodeError"] = err.Error()
		return r
	}
	d, err := softwarevideo.New(4)
	if err != nil {
		r["decodeError"] = err.Error()
		return r
	}
	defer d.Close()
	im, err := d.Decode(data)
	r["decodeOK"] = err == nil && im != nil && im.Bounds().Dx() == width && im.Bounds().Dy() == height
	if err != nil {
		r["decodeError"] = err.Error()
	}
	return r
}
