//go:build windows && cgo && ffmpeg

package video

import (
	"testing"
)

// 實機 Smoke：強制 FFmpeg CPU，完全不依賴系統 codec extension。
func TestWindowsSoftwareDecoderSmoke(t *testing.T) {
	for _, codec := range []WireCodec{WireH264, WireHEVC, WireAV1} {
		t.Run(decodePolicyCodec(codec), func(t *testing.T) {
			d := &windowsDecoder{software: true, codec: codec}
			defer d.Close()
			fixture := probeH264
			if codec == WireAV1 {
				fixture = probeAV1
			}
			if codec == WireHEVC {
				fixture = probeHEVC
			}
			for frame := 0; frame < 3; frame++ {
				im, err := d.Decode(fixture)
				if err != nil {
					t.Fatalf("FFmpeg 軟解未通過：%v", err)
				}
				if im == nil || im.Bounds().Dx() != 128 || im.Bounds().Dy() != 128 || d.DecodingMode() != "software" {
					t.Fatal("軟解影格或模式不正確")
				}
			}
		})
	}
}

// 即使 AV1／HEVC 沒有軟體編碼器，解碼工作仍必須通過，且不得帶入編碼結果。
func TestIndependentSoftwareDecodeProbe(t *testing.T) {
	for _, codec := range []string{"jpeg", "h264", "hevc", "av1"} {
		for _, size := range [][2]int{{128, 128}, {1920, 1080}} {
			r := ProbeSoftwareCodec(codec, size[0], size[1], "decode")
			if r["decodeOK"] != true || r["hardwareDecoder"] != false {
				t.Fatalf("%s %v: %+v", codec, size, r)
			}
			if _, exists := r["encodeOK"]; exists {
				t.Fatal("解碼工作包含編碼結果")
			}
		}
	}
}
