//go:build windows

package video

func detectCapabilities() capabilities {
	result := capabilities{backend: "Media Foundation / D3D11", detail: "以實際編碼影格驗證硬體能力；失敗使用 JPEG 備援"}
	for _, cap := range IntraCapabilities() {
		if cap.Codec == CodecHardwareH264 {
			result.h264 = cap.Encode
		}
		if cap.Codec == CodecHardwareHEVC {
			result.hevc = cap.Encode
		}
	}
	return result
}
