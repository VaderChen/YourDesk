package video

import (
	"runtime"
	"sync"
	"yourdesk/internal/optimization"
)

var decodeOnce sync.Once
var decodeReady = make(chan struct{})
var decodeCaps []IntraCapability

func startDecodeProbe() { decodeOnce.Do(func() { go probeDecodeCapabilities() }) }

// CachedDecodeCapabilities 只驗證觀看端需要的解碼；與完整能力查詢共用快取。
func CachedDecodeCapabilities() []IntraCapability {
	startDecodeProbe()
	select {
	case <-decodeReady:
		return append([]IntraCapability(nil), decodeCaps...)
	default:
		return nil
	}
}

func probeDecodeCapabilities() {
	defer close(decodeReady)
	p := optimization.Snapshot()
	codecs := []Codec{CodecHardwareH264, CodecHardwareHEVC}
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		codecs = append(codecs, CodecHardwareAV1)
	}
	for _, codec := range codecs {
		cap := IntraCapability{Codec: codec}
		mode := p.DecoderDecision(decodePolicyCodec(WireForCodec(codec)), 128, 128)
		if mode == "hardware" || mode == "auto" {
			cap.Decode = true
			cap.DecodeMode = mode
		} else if mode == "unavailable" && runtime.GOOS != "windows" {
			cap.DecodeError = "本機偵測已確認此尺寸解碼不可用"
		} else {
			// 獨立 fixture，不依賴本機能否編碼；硬解失敗仍允許原有軟解。
			fixture := probeH264
			if codec == CodecHardwareAV1 {
				fixture = probeAV1
			}
			if codec == CodecHardwareHEVC {
				fixture = probeHEVC
			}
			d, err := newIntraDecoder(WireForCodec(codec))
			if err != nil {
				cap.DecodeError = err.Error()
			} else {
				im, e := d.Decode(fixture)
				cap.Decode = e == nil && im != nil && im.Bounds().Dx() == 128 && im.Bounds().Dy() == 128
				if e != nil {
					cap.DecodeError = e.Error()
				}
				if cap.Decode {
					if reporter, ok := d.(interface{ DecodingMode() string }); ok {
						cap.DecodeMode = reporter.DecodingMode()
					}
				}
				d.Close()
			}
		}
		decodeCaps = append(decodeCaps, cap)
	}
}
