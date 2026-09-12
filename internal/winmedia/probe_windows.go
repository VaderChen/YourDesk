//go:build windows && cgo

package winmedia

/*
#include "native.h"
*/
import "C"

import (
	"fmt"
	"runtime"
	"time"
	"unsafe"
)

// Probe 僅由受逾時保護的 helper 使用；COM 資源全程固定於同一執行緒。
func Probe(codec, format string, width, height int) map[string]any {
	return probeCodec(codec, format, width, height, false)
}
func ProbeSoftware(codec, format string, width, height int) map[string]any {
	return probeCodec(codec, format, width, height, true)
}
func probeCodec(codec, format string, width, height int, softwareOnly bool) map[string]any {
	start := time.Now()
	r := map[string]any{"probeKind": "windows-native", "codec": codec, "input": format, "output": format, "width": width, "height": height}
	codecs := map[string]int{"jpeg": 0, "h264": 1, "hevc": 2, "av1": 3}
	formats := map[string]int{"BGRA": 0, "NV12-full": 1, "NV12-video": 2}
	c, validCodec := codecs[codec]
	f, validFormat := formats[format]
	if !validCodec || !validFormat {
		r["status"] = "invalid-request"
		return r
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var out C.yd_media_probe_result
	var hr C.int
	if softwareOnly {
		hr = C.yd_media_probe_software(C.int(c), C.int(f), C.int(width), C.int(height), &out)
	} else {
		hr = C.yd_media_probe(C.int(c), C.int(f), C.int(width), C.int(height), &out)
	}
	r["durationMS"] = float64(time.Since(start).Microseconds()) / 1000
	r["encodeOK"] = out.encode_ok != 0
	r["encode_status"] = int32(out.encode_status)
	r["hardwareEncoder"] = probeHardware(out.hardware_encoder)
	r["encoderBackend"] = C.GoString(&out.encoder[0])
	if out.encoder_d3d11 != 0 {
		r["encoderGraphicsAPI"] = "D3D11"
	}
	r["inputNominalRange"] = int(out.input_range)
	r["outputNominalRange"] = int(out.output_range)
	r["decoderAttempted"] = out.decoder_attempted != 0
	if out.decoder_attempted != 0 {
		r["decodeOK"] = out.decode_ok != 0
		r["decode_status"] = int32(out.decode_status)
		r["hardwareDecoder"] = probeHardware(out.hardware_decoder)
		r["decoderBackend"] = C.GoString(&out.decoder[0])
		if out.decoder_d3d11 != 0 {
			r["decoderGraphicsAPI"] = "D3D11"
		}
	}
	if hr < 0 {
		r["nativeStatus"] = int32(hr)
		if out.encode_ok == 0 {
			r["encode_status"] = int32(hr)
		} else if out.decode_ok == 0 {
			r["decode_status"] = int32(hr)
		}
	}
	return r
}
func probeHardware(value C.int) any {
	if value < 0 {
		return nil
	}
	return value != 0
}

// ProbeGraphics 與編解碼證據分開；建立 D3D 裝置成功不代表支援某個 codec。
func ProbeGraphics() []map[string]any {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var devices []map[string]any
	for i := 0; i < 32; i++ {
		var out C.yd_graphics_probe_result
		status := C.yd_graphics_probe(C.uint(i), &out)
		if status < 0 || status == 1 {
			break
		}
		if status == 2 {
			continue
		}
		devices = append(devices, map[string]any{
			"name":  C.GoString(&out.name[0]),
			"d3d11": graphicsSupport(int32(out.d3d11_status)), "d3d12": graphicsSupport(int32(out.d3d12_status)),
			"d3d11Status": int32(out.d3d11_status), "d3d12Status": int32(out.d3d12_status),
			"d3d11FeatureLevel": featureLevel(uint32(out.d3d11_level)), "d3d12FeatureLevel": featureLevel(uint32(out.d3d12_level)),
		})
	}
	return devices
}
func featureLevel(level uint32) string {
	if level == 0 {
		return ""
	}
	return fmt.Sprintf("%d.%d", level>>12, (level>>8)&15)
}

// 僅明確的 API／功能不支援才允許降級；裝置移除、記憶體不足等失敗保留未知。
func graphicsSupport(status int32) any {
	if status >= 0 {
		return true
	}
	switch uint32(status) {
	case 0x80004002, 0x887a0004, 0x8007007e, 0x8007007f, 0x80004001:
		return false
	default:
		return nil
	}
}

// ProbeDecode 只建立解碼器，不建立編碼器或使用本機編碼結果。
func ProbeDecode(codec, format string, width, height int, fixture []byte) map[string]any {
	start := time.Now()
	r := map[string]any{"probeKind": "windows-native", "codec": codec, "input": format, "output": format, "width": width, "height": height, "phase": "decode", "decoderSource": "independent-fixture", "decoderAttempted": true, "decodeOK": false}
	c, ok := map[string]int{"jpeg": 0, "h264": 1, "hevc": 2, "av1": 3}[codec]
	f, valid := map[string]int{"BGRA": 0, "NV12-full": 1, "NV12-video": 2}[format]
	if !ok || !valid || len(fixture) == 0 {
		r["status"] = "invalid-request"
		return r
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var out C.yd_media_probe_result
	hr := C.yd_media_probe_decode(C.int(c), C.int(f), C.int(width), C.int(height), (*C.uchar)(unsafe.Pointer(&fixture[0])), C.size_t(len(fixture)), &out)
	r["decodeOK"] = out.decode_ok != 0
	r["decode_status"] = int32(out.decode_status)
	if hr < 0 {
		r["decode_status"] = int32(hr)
	}
	r["hardwareDecoder"] = probeHardware(out.hardware_decoder)
	r["decoderBackend"] = C.GoString(&out.decoder[0])
	r["outputNominalRange"] = int(out.output_range)
	if out.decoder_d3d11 != 0 {
		r["decoderGraphicsAPI"] = "D3D11"
	}
	r["durationMS"] = float64(time.Since(start).Microseconds()) / 1000
	return r
}
