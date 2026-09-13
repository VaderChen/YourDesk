package hardwareprobe

import (
	"encoding/json"
	"runtime"
	"yourdesk/internal/optimization"
)

// Policy 僅由已完成的證據建立；pending／timeout／未知不當成硬體不支援。
func Policy() *optimization.Policy { return policyFromState(Snapshot()) }
func policyFromState(s State) *optimization.Policy {
	if s.Status != "complete" && s.Status != "partial" {
		return nil
	}
	p := &optimization.Policy{Version: 1, OS: runtime.GOOS, Arch: runtime.GOARCH}
	for _, r := range s.Results {
		if r.State != "complete" {
			continue
		}
		var m map[string]any
		if json.Unmarshal(r.Data, &m) != nil {
			continue
		}
		if r.Key == "inventory" {
			features, _ := m["cpuFeatures"].(map[string]any)
			native, _ := m["cpu"].(map[string]any)
			for _, name := range []string{"SSE2", "SSSE3", "AVX2", "ASIMD"} {
				if truth(features[name]) {
					p.CompareBytes = 64 << 20
				}
			}
			for _, name := range []string{"hw.optional.neon", "hw.optional.sse2", "hw.optional.avx2_0"} {
				if truth(native[name]) {
					p.CompareBytes = 64 << 20
				}
			}
			if memory, ok := native["hw.memsize"].(float64); ok && memory < 4<<30 {
				p.CompareBytes = min(p.CompareBytes, 16<<20)
			}
			continue
		}
		// 原生格式矩陣只提供診斷證據；正式路徑仍以 RGBA 實測產生策略。
		if m["probeKind"] == "windows-native" {
			continue
		}
		codec, _ := m["codec"].(string)
		if codec != "h264" && codec != "hevc" && codec != "av1" {
			continue
		}
		if codec == "av1" && m["probeKind"] == "software" {
			if ok, present := m["encodeOK"].(bool); present {
				w, _ := m["width"].(float64)
				h, _ := m["height"].(float64)
				p.Encoders = append(p.Encoders, optimization.Encoder{Codec: "software-av1", Width: int(w), Height: int(h), Usable: ok})
			}
		}
		format, _ := m["input"].(string)
		if format != "BGRA" && format != "RGBA" {
			continue
		}
		w, _ := m["width"].(float64)
		h, _ := m["height"].(float64)
		usable, known := false, false
		if format == "BGRA" {
			hardware, _ := m["hardware_encoder"].(map[string]any)
			usable = zero(m, "create_status") && zero(m, "encode_status") && zero(m, "encode_callback_status") && truth(m["sample_produced"]) && truth(hardware["value"])
			known = usable || nonzero(m, "create_status") || nonzero(m, "encode_status") || nonzero(m, "encode_callback_status")
		} else {
			usable = truth(m["encodeOK"]) && truth(m["hardwareEncoder"])
			_, hasEncode := m["encodeOK"]
			known = usable || (hasEncode && !truth(m["encodeOK"])) || m["status"] == "unavailable"
		}
		if known {
			p.Encoders = append(p.Encoders, optimization.Encoder{Codec: codec, Width: int(w), Height: int(h), Usable: usable})
		}
		// 編碼失敗而未執行解碼，不構成解碼器不可用的證據。
		mode := ""
		if format == "BGRA" {
			hardware, _ := m["hardware_decoder"].(map[string]any)
			if zero(m, "decoder_create_status") && zero(m, "decode_status") && zero(m, "decode_callback_status") && truth(m["decoded_size_matches"]) && (m["decodeOK"] == nil || truth(m["decodeOK"])) && truth(hardware["value"]) {
				mode = "hardware"
			} else if nonzero(m, "decoder_create_status") || nonzero(m, "decode_status") || nonzero(m, "decode_callback_status") {
				// 原生探測要求硬解；失敗只避開硬解，不否定軟解。
				mode = "software"
			}
		} else if decoded, present := m["decodeOK"].(bool); present {
			if decoded {
				mode = "auto"
				if m["decodingMode"] == "software" {
					mode = "software"
				}
				if m["decodingMode"] == "hardware" {
					mode = "hardware"
				}
			} else {
				mode = "unavailable"
			}
		}
		if mode != "" {
			p.Decoders = append(p.Decoders, optimization.Decoder{Codec: codec, Width: int(w), Height: int(h), Mode: mode})
		}
	}
	// 獨立 CPU 成功證據可補足編碼失敗時未執行的解碼；不能蓋過硬解成功證據。
	for _, r := range s.Results {
		if r.State != "complete" {
			continue
		}
		var m map[string]any
		if json.Unmarshal(r.Data, &m) != nil || (m["probeKind"] != "windows-software" && m["probeKind"] != "software") || !truth(m["decodeOK"]) {
			continue
		}
		codec, _ := m["codec"].(string)
		if codec != "h264" && codec != "hevc" && codec != "av1" {
			continue
		}
		w, _ := m["width"].(float64)
		h, _ := m["height"].(float64)
		found := false
		for i, d := range p.Decoders {
			if d.Codec == codec && d.Width == int(w) && d.Height == int(h) {
				found = true
				if d.Mode == "unavailable" {
					p.Decoders[i].Mode = "software"
				}
				break
			}
		}
		if !found {
			p.Decoders = append(p.Decoders, optimization.Decoder{Codec: codec, Width: int(w), Height: int(h), Mode: "software"})
		}
	}
	return p
}
func truth(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case float64:
		return x == 1
	}
	return false
}
func zero(m map[string]any, k string) bool    { v, ok := m[k].(float64); return ok && v == 0 }
func nonzero(m map[string]any, k string) bool { v, ok := m[k].(float64); return ok && v != 0 }
