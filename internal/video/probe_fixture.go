package video

import (
	"embed"
	"fmt"
)

//go:embed probes/av1-128.obu probes/av1-1080.obu probes/jpeg-128.jpg probes/jpeg-1920.jpg probes/h264.bin probes/hevc.bin probes/h264-1080.bin probes/hevc-1080.bin
var nativeProbeFixtures embed.FS

// ProbeWireFixture 使用固定壓縮影格，解碼測試不會建立編碼器。
func ProbeWireFixture(codec string, width, height int) ([]byte, error) {
	if !((width == 128 && height == 128) || (width == 1920 && height == 1080)) {
		return nil, fmt.Errorf("無對應尺寸的解碼樣本")
	}
	var name string
	switch codec {
	case "av1":
		name = "av1-128.obu"
		if width == 1920 {
			name = "av1-1080.obu"
		}
	case "jpeg":
		name = fmt.Sprintf("jpeg-%d.jpg", width)
	case "h264", "hevc":
		name = codec + ".bin"
		if width == 1920 {
			name = codec + "-1080.bin"
		}
	default:
		return nil, fmt.Errorf("無此格式的解碼樣本")
	}
	return nativeProbeFixtures.ReadFile("probes/" + name)
}
