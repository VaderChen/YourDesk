//go:build windows && cgo

package video

import (
	_ "embed"
	"image"

	"time"
	"yourdesk/internal/softwarevideo"
	"yourdesk/internal/winmedia"
)

//go:embed probes/h264-1080.annexb
var softwareH2641080 []byte

//go:embed probes/hevc-1080.annexb
var softwareHEVC1080 []byte

//go:embed probes/av1-1080.obu
var softwareAV11080 []byte

// ProbeSoftwareCodec 每次只測一個方向；解碼僅讀取固定樣本。
func ProbeSoftwareCodec(codec string, width, height int, phase string) map[string]any {
	if codec == "av1" {
		return ProbeSoftwareAV1(width, height, phase)
	}
	start := time.Now()
	r := map[string]any{"probeKind": "windows-software", "codec": codec, "input": "software", "width": width, "height": height, "phase": phase}
	defer func() { r["durationMS"] = float64(time.Since(start).Microseconds()) / 1000 }()
	fixture, err := ProbeElementaryFixture(codec, width, height)
	if err != nil || (phase != "encode" && phase != "decode") {
		r["status"] = "invalid-request"
		return r
	}
	if phase == "encode" {
		if codec == "jpeg" {
			src := image.NewRGBA(image.Rect(0, 0, width, height))
			for i := 3; i < len(src.Pix); i += 4 {
				src.Pix[i] = 255
			}
			enc, _ := NewJPEGEncoder(false)
			defer enc.Close()
			encoded, e := enc.Encode(src, 70)
			r["encodeOK"] = e == nil && len(encoded) > 0
			r["hardwareEncoder"] = false
			r["encoderBackend"] = enc.Backend()

		} else {
			for k, v := range winmedia.ProbeSoftware(codec, "NV12-video", width, height) {
				r[k] = v
			}
			r["probeKind"] = "windows-software"
			r["input"] = "software"
			r["encoderInput"] = "NV12-video"
		}
		return r
	}
	r["decoderAttempted"] = true
	r["decoderSource"] = "independent-fixture"
	r["output"] = "RGBA"
	r["decodeOK"] = false
	r["hardwareDecoder"] = false
	r["decodingMode"] = "software"
	if codec == "jpeg" {
		dec, _ := NewJPEGDecoder(false)
		defer dec.Close()
		im, e := dec.Decode(fixture)
		err = e
		r["decodeOK"] = e == nil && im != nil && im.Bounds().Dx() == width && im.Bounds().Dy() == height
		r["decoderBackend"] = dec.Backend()
	} else {
		kind := map[string]int{"h264": 1, "hevc": 2, "av1": 3}[codec]
		var dec *softwarevideo.Decoder
		dec, err = softwarevideo.New(kind)
		if err == nil {
			defer dec.Close()
			im, e := dec.Decode(fixture)
			err = e
			r["decodeOK"] = e == nil && im != nil && im.Bounds().Dx() == width && im.Bounds().Dy() == height
			r["decoderBackend"] = dec.Backend()
		}
	}
	if err != nil {
		r["decodeError"] = err.Error()
	}
	return r
}
