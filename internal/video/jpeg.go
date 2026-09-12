package video

import "sync"

// JPEGCapabilities describes only a runtime-probed hardware JPEG path.
// Merely having a framework or GPU is not sufficient to set Encode/Decode.
type JPEGCapabilities struct {
	Encode  bool
	Decode  bool
	Backend string
	Detail  string
	Probed  bool
}

var (
	jpegWarmOnce sync.Once
	jpegProbeMu  sync.Mutex
	jpegProbe    func() JPEGCapabilities
	jpegCached   *JPEGCapabilities
)

// CachedHardwareJPEG 給 UI 使用：快取鎖被原生查詢持有時直接回報待偵測，絕不等待。
func CachedHardwareJPEG() JPEGCapabilities {
	if jpegProbeMu.TryLock() {
		if jpegCached != nil {
			result := *jpegCached
			jpegProbeMu.Unlock()
			return result
		}
		jpegProbeMu.Unlock()
	}
	jpegWarmOnce.Do(func() { go ProbeHardwareJPEG() })
	return JPEGCapabilities{Backend: "pending", Detail: "背景偵測中，暫用既有安全後端"}
}

// RegisterHardwareJPEGProbe is used by a platform build-tag implementation.
// It is intentionally internal-facing so a failed probe can be cached and
// cannot silently change codec choice halfway through a session.
func RegisterHardwareJPEGProbe(probe func() JPEGCapabilities) {
	jpegProbeMu.Lock()
	jpegProbe = probe
	jpegCached = nil
	jpegProbeMu.Unlock()
}

func ProbeHardwareJPEG() JPEGCapabilities {
	jpegProbeMu.Lock()
	defer jpegProbeMu.Unlock()
	if jpegCached != nil {
		return *jpegCached
	}
	result := JPEGCapabilities{Backend: "none", Detail: "沒有可用的硬體 JPEG runtime probe", Probed: true}
	if jpegProbe != nil {
		result = jpegProbe()
		result.Probed = true
	}
	jpegCached = &result
	return result
}

func SelectJPEGEncoder(requested Codec) Selection {
	if requested == CodecHardwareJPEG {
		c := ProbeHardwareJPEG()
		if c.Encode {
			return Selection{Requested: requested, Selected: CodecHardwareJPEG, Backend: c.Backend, Hardware: true, Detail: c.Detail}
		}
		return Selection{Requested: requested, Selected: CodecSoftwareJPEG, Backend: preferredSoftwareJPEGBackend, Detail: "硬體 JPEG probe 失敗，退回軟體 JPEG"}
	}
	if requested == CodecAuto {
		c := ProbeHardwareJPEG()
		if c.Encode {
			return Selection{Requested: requested, Selected: CodecHardwareJPEG, Backend: c.Backend, Hardware: true, Detail: c.Detail}
		}
	}
	return Selection{Requested: requested, Selected: CodecSoftwareJPEG, Backend: preferredSoftwareJPEGBackend, Detail: "使用軟體 JPEG"}
}

func SelectJPEGDecoder(requested string) DecoderSelection {
	if requested == "software" {
		return DecoderSelection{Hardware: false, Backend: "software", Codec: CodecSoftwareJPEG, Probed: true, Detail: "明確指定軟體 JPEG 解碼"}
	}
	c := ProbeHardwareJPEG()
	if c.Decode {
		return DecoderSelection{Hardware: true, Backend: c.Backend, Codec: CodecHardwareJPEG, Probed: true, Detail: c.Detail}
	}
	return DecoderSelection{Hardware: false, Backend: "software", Codec: CodecSoftwareJPEG, Probed: true, Detail: "硬體 JPEG decoder probe 失敗，退回軟體"}
}
