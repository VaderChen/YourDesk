//go:build windows && cgo

package video

import (
	_ "embed"
	"fmt"
)

//go:embed probes/jpeg-128.jpg
var probeJPEG128 []byte

//go:embed probes/jpeg-1920.jpg
var probeJPEG1080 []byte

// ProbeElementaryFixture 提供預先建立的壓縮樣本，不呼叫任何待測編碼器。
func ProbeElementaryFixture(codec string, width, height int) ([]byte, error) {
	if !((width == 128 && height == 128) || (width == 1920 && height == 1080)) {
		return nil, fmt.Errorf("無對應尺寸的解碼樣本")
	}
	switch codec {
	case "jpeg":
		if width == 128 {
			return probeJPEG128, nil
		}
		return probeJPEG1080, nil
	case "h264":
		if width == 128 {
			return unpackH264(probeH264)
		}
		return softwareH2641080, nil
	case "hevc":
		if width == 128 {
			return unpackHEVC(probeHEVC)
		}
		return softwareHEVC1080, nil
	case "av1":
		if width == 128 {
			return probeAV1, nil
		}
		return softwareAV11080, nil
	}
	return nil, fmt.Errorf("無此格式的解碼樣本")
}
