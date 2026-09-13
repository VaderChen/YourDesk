package video

import (
	_ "embed"
	"errors"
	"image"
	"log/slog"
	"sync"
	"yourdesk/internal/optimization"
)

// WireCodec 使用影格標頭的保留位元組；0 保留給舊版 JPEG。
type WireCodec byte

const (
	WireJPEG WireCodec = iota
	WireH264
	WireHEVC
	WireAV1
)

// IntraEncoder 保留既有名稱；協商後可輸出含參考影格的 GOP。
type IntraEncoder interface {
	Encode(image.Image, int) ([]byte, error)
	Close() error
}

// GOPEncoder 設定 IDR 間隔，預設 1 以相容舊 遠端顯示。
type GOPEncoder interface{ SetKeyframeInterval(int) }

// RateEncoder 設定平均位元率及影格時間；零位元率使用編碼器預設值。
type RateEncoder interface {
	SetRate(bitsPerSecond, fps int)
}

var ErrVideoUnavailable = errors.New("硬體影像編碼不可用")

type IntraCapability struct {
	Codec       Codec  `json:"codec"`
	Encode      bool   `json:"encode"`
	Decode      bool   `json:"decode"`
	DecodeMode  string `json:"decodeMode,omitempty"`
	EncodeError string `json:"encodeError,omitempty"`
	DecodeError string `json:"decodeError,omitempty"`
}

var intraOnce sync.Once
var intraCaps []IntraCapability
var intraReady = make(chan struct{})

// 探測用固定黑色影格由 FFmpeg 產生，解碼能力不依賴本機編碼器。
//
//go:embed probes/h264.bin
var probeH264 []byte

//go:embed probes/hevc.bin
var probeHEVC []byte

//go:embed probes/av1-128.obu
var probeAV1 []byte

func startIntraProbe() {
	intraOnce.Do(func() { go probeIntraCapabilities() })
}

// UI 僅讀取已完成的快取，首次呼叫啟動背景探測。
func CachedIntraCapabilities() []IntraCapability {
	startIntraProbe()
	select {
	case <-intraReady:
		return append([]IntraCapability(nil), intraCaps...)
	default:
		return nil
	}
}
func IntraCapabilities() []IntraCapability {
	startIntraProbe()
	<-intraReady
	return append([]IntraCapability(nil), intraCaps...)
}
func probeIntraCapabilities() {
	defer close(intraReady)
	startDecodeProbe()
	<-decodeReady
	policy := optimization.Snapshot()
	caps := append([]IntraCapability(nil), decodeCaps...)
	for _, cap := range decodeCaps {
		if cap.Codec == CodecHardwareAV1 {
			cap.Codec = CodecSoftwareAV1
			caps = append(caps, cap)
		}
	}
	for _, cap := range caps {
		codec := cap.Codec
		if usable, known := policy.EncoderDecision(decodePolicyCodec(WireForCodec(codec)), 128, 128); known && codec != CodecSoftwareAV1 {
			cap.Encode = usable
			if !usable {
				cap.EncodeError = "本機偵測已確認此尺寸編碼不可用"
			}
			intraCaps = append(intraCaps, cap)
			continue
		}
		enc, err := NewIntraEncoder(codec)
		if err == nil {
			pixels := image.NewRGBA(image.Rect(0, 0, 128, 128))
			for i := 3; i < len(pixels.Pix); i += 4 {
				pixels.Pix[i] = 255
			}
			payload, encodeErr := enc.Encode(pixels, 70)
			if encodeErr != nil {
				cap.EncodeError = encodeErr.Error()
			}
			cap.Encode = encodeErr == nil && len(payload) > 0
			if cap.Encode && codec == CodecHardwareH264 {
				_, wireErr := unpackH264(payload)
				cap.Encode = wireErr == nil
				if wireErr != nil {
					cap.EncodeError = wireErr.Error()
				}
			}
			if reporter, ok := enc.(BackendReporter); ok && cap.Encode {
				slog.Info("硬體編碼能力探測通過", "codec", codec, "backend", reporter.Backend())
			}
			enc.Close()
		} else {
			cap.EncodeError = err.Error()
		}
		if !cap.Encode {
			slog.Warn("影片編碼能力探測未通過", "codec", codec, "error", cap.EncodeError)
		}
		if !cap.Decode {
			slog.Warn("影片解碼能力探測未通過", "codec", codec, "error", cap.DecodeError)
		}
		intraCaps = append(intraCaps, cap)
	}
}
func WireForCodec(codec Codec) WireCodec {
	if codec == CodecHardwareAV1 || codec == CodecSoftwareAV1 {
		return WireAV1
	}
	if codec == CodecHardwareH264 {
		return WireH264
	}
	if codec == CodecHardwareHEVC {
		return WireHEVC
	}
	return WireJPEG
}
func SupportsIntra(codec Codec) bool {
	for _, cap := range IntraCapabilities() {
		if cap.Codec == codec {
			return cap.Encode
		}
	}
	return false
}
