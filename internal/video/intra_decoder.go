package video

import (
	"errors"
	"image"
	"runtime"
	"sync"
	"yourdesk/internal/optimization"
)

// IntraDecoder 讓平台保留解碼工作階段，避免每張影格重新建立 COM／D3D 資源。
type IntraDecoder interface {
	Decode([]byte) (image.Image, error)
	Close() error
}
type BackendReporter interface{ Backend() string }
type DecodeSession struct {
	mu      sync.Mutex
	decoder IntraDecoder
	codec   WireCodec
}

func (s *DecodeSession) Decode(codec WireCodec, payload []byte) (image.Image, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, err := IsKeyframe(codec, payload)
	if err != nil {
		return nil, err
	}
	if (s.decoder == nil || s.codec != codec) && !key {
		return nil, ErrNeedKeyframe
	}
	if s.decoder == nil || s.codec != codec {
		if s.decoder != nil {
			s.decoder.Close()
			s.decoder = nil
		}
		var err error
		s.decoder, err = newIntraDecoder(codec)
		if err != nil {
			return nil, err
		}
		s.codec = codec
		if configurable, ok := s.decoder.(interface{ SetDecodePolicy(optimization.Policy) }); ok {
			configurable.SetDecodePolicy(optimization.Snapshot())
		}
	}
	img, err := s.decoder.Decode(payload)
	if err != nil && !errors.Is(err, ErrNeedKeyframe) {
		s.decoder.Close()
		s.decoder = nil
	}
	return img, err
}

func decodePolicyCodec(codec WireCodec) string {
	switch codec {
	case WireAV1:
		return "av1"
	case WireH264:
		return "h264"
	case WireHEVC:
		return "hevc"
	default:
		return ""
	}
}

// ReceiverCodecs 不等待探測。先採已驗證的解碼結果，其餘使用既有背景快取。
// 128×128 只作格式協商，實際尺寸仍由平台解碼器逐一驗證。
func ReceiverCodecs() []byte {
	p := optimization.Snapshot()
	var codecs []byte
	var cached []IntraCapability
	loaded := false
	for _, codec := range receiverWireCodecs() {
		mode := p.DecoderDecision(decodePolicyCodec(codec), 128, 128)
		switch mode {
		case "hardware", "auto":
			codecs = append(codecs, byte(codec))
		case "unavailable":
			if runtime.GOOS != "windows" {
				continue
			}
			fallthrough
		default:
			if !loaded {
				cached = CachedDecodeCapabilities()
				loaded = true
			}
			for _, cap := range cached {
				if cap.Decode && WireForCodec(cap.Codec) == codec {
					codecs = append(codecs, byte(codec))
				}
			}
		}
	}
	return codecs
}
func (s *DecodeSession) Backend() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if value, ok := s.decoder.(BackendReporter); ok {
		return value.Backend()
	}
	return "VideoToolbox 自動選擇解碼"
}
func (s *DecodeSession) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.decoder != nil {
		s.decoder.Close()
		s.decoder = nil
	}
}

// 解碼模式只取自目前成功使用的工作階段。
func (s *DecodeSession) DecodingMode() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d, ok := s.decoder.(interface{ DecodingMode() string }); ok {
		return d.DecodingMode()
	}
	return "unknown"
}

// 軟解亦可公告 codec；硬解清單僅用於同等編碼候選的優先排序。
func ReceiverHardwareCodecs(available []byte) []byte {
	p := optimization.Snapshot()
	cached := CachedDecodeCapabilities()
	var hardware []byte
	for _, value := range available {
		codec := WireCodec(value)
		mode := p.DecoderDecision(decodePolicyCodec(codec), 128, 128)
		if mode == "hardware" {
			hardware = append(hardware, value)
			continue
		}
		if mode == "unavailable" {
			continue
		}
		for _, cap := range cached {
			if cap.Decode && cap.DecodeMode == "hardware" && WireForCodec(cap.Codec) == codec {
				hardware = append(hardware, value)
				break
			}
		}
	}
	return hardware
}

// 同時具備硬編／硬解的候選先行，其餘硬編可搭配軟解；同級優先 HEVC。
func PreferredHardwareEncoders(remoteHardware uint32) []Codec {
	result := make([]Codec, 0, 2)
	for _, hardware := range []bool{true, false} {
		candidates := []Codec{CodecHardwareHEVC, CodecHardwareH264}
		if runtime.GOOS == "windows" {
			candidates = append(candidates, CodecHardwareAV1)
		}
		for _, codec := range candidates {
			if (remoteHardware&(1<<WireForCodec(codec)) != 0) == hardware {
				result = append(result, codec)
			}
		}
	}
	return result
}

func receiverWireCodecs() []WireCodec {
	codecs := []WireCodec{WireH264, WireHEVC}
	if runtime.GOOS == "windows" {
		codecs = append(codecs, WireAV1)
	}
	return codecs
}
