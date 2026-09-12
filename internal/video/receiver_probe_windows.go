//go:build windows && cgo

package video

import (
	_ "embed"
	"time"
)

// 與編碼測試分離，直接餵入完整 wire packet，包含參數集及壓縮影格。
//
//go:embed probes/h264-1080.bin
var receiverH2641080 []byte

//go:embed probes/hevc-1080.bin
var receiverHEVC1080 []byte

// ProbeReceiverCodec 走正式接收端（硬解優先、FFmpeg 備援），不依賴本機編碼。
func ProbeReceiverCodec(codec string, width, height int) map[string]any {
	start := time.Now()
	result := map[string]any{"decoderAttempted": true, "decoderSource": "independent-fixture", "decoderOutput": "RGBA", "decodeOK": false}
	defer func() { result["durationMS"] = float64(time.Since(start).Microseconds()) / 1000 }()
	if codec == "jpeg" {
		fixture, err := ProbeElementaryFixture(codec, width, height)
		if err != nil {
			result["decodeError"] = err.Error()
			return result
		}
		dec, _ := NewJPEGDecoder(true)
		defer dec.Close()
		im, err := dec.Decode(fixture)
		result["decodeOK"] = err == nil && im != nil && im.Bounds().Dx() == width && im.Bounds().Dy() == height
		result["hardwareDecoder"] = dec.Hardware()
		result["decoderBackend"] = dec.Backend()
		if err != nil {
			result["decodeError"] = err.Error()
		}
		return result
	}
	if !((width == 128 && height == 128) || (width == 1920 && height == 1080)) {
		result["decodeError"] = "無對應尺寸的解碼 fixture"
		return result
	}
	var wire WireCodec
	var data []byte
	switch codec {
	case "h264":
		wire = WireH264
		data = probeH264
		if width == 1920 {
			data = receiverH2641080
		}
	case "hevc":
		wire = WireHEVC
		data = probeHEVC
		if width == 1920 {
			data = receiverHEVC1080
		}
	case "av1":
		wire = WireAV1
		data = probeAV1
		if width == 1920 {
			data = softwareAV11080
		}
	default:
		result["decodeError"] = "無此格式的解碼 fixture"
		return result
	}
	var session DecodeSession
	defer session.Close()
	im, err := session.Decode(wire, data)
	if err != nil {
		result["decodeError"] = err.Error()
		return result
	}
	if im == nil || im.Bounds().Dx() != width || im.Bounds().Dy() != height {
		result["decodeError"] = "解碼輸出尺寸不符"
		return result
	}
	result["decodeOK"] = true
	result["decoderBackend"] = session.Backend()
	mode := session.DecodingMode()
	result["decodingMode"] = mode
	if mode == "hardware" || mode == "software" {
		result["hardwareDecoder"] = mode == "hardware"
	}
	return result
}
