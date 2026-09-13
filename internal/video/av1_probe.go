package video

import (
	"image"
	"time"
	"yourdesk/internal/softwarevideo"
)

// ProbeSoftwareAV1 的解碼使用固定樣本，不取用本輪編碼的輸出。
func ProbeSoftwareAV1(width, height int, phase string) map[string]any {
	start := time.Now()
	r := map[string]any{"probeKind": "software", "codec": "av1", "input": "software", "width": width, "height": height, "phase": phase}
	defer func() { r["durationMS"] = float64(time.Since(start).Microseconds()) / 1000 }()
	fixture, err := ProbeWireFixture("av1", width, height)
	if err != nil || (phase != "encode" && phase != "decode") {
		r["status"] = "invalid-request"
		return r
	}
	if phase == "encode" {
		r["encodeOK"] = false
		r["hardwareEncoder"] = false
		r["encoderBackend"] = "FFmpeg / libaom AV1 CPU"
		e, eerr := newSoftwareAV1Encoder()
		if eerr != nil {
			r["encodeError"] = eerr.Error()
			return r
		}
		defer e.Close()
		src := image.NewRGBA(image.Rect(0, 0, width, height))
		for i := 3; i < len(src.Pix); i += 4 {
			src.Pix[i] = 255
		}
		payload, eerr := e.Encode(src, 70)
		if eerr == nil {
			_, eerr = IsKeyframe(WireAV1, payload)
		}
		r["encodeOK"] = eerr == nil && len(payload) > 0
		if eerr != nil {
			r["encodeError"] = eerr.Error()
		}
	} else {
		r["decodeOK"] = false
		r["hardwareDecoder"] = false
		r["decodingMode"] = "software"
		r["decoderSource"] = "independent-fixture"
		d, e := softwarevideo.New(3)
		if e != nil {
			r["decodeError"] = e.Error()
			return r
		}
		defer d.Close()
		r["decoderBackend"] = d.Backend()
		im, e := d.Decode(fixture)
		r["decodeOK"] = e == nil && im != nil && im.Bounds().Dx() == width && im.Bounds().Dy() == height
		if e != nil {
			r["decodeError"] = e.Error()
		}
	}
	return r
}
