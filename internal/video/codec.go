package video

import (
	"fmt"
)

type Codec string

const (
	CodecSoftwareAV1  Codec = "software-av1"
	CodecHardwareAV1  Codec = "hardware-av1"
	CodecAuto         Codec = "auto"
	CodecHardwareH264 Codec = "hardware-h264"
	CodecHardwareHEVC Codec = "hardware-hevc"
	CodecSoftwareH264 Codec = "software-h264"
	CodecHardwareJPEG Codec = "hardware-jpeg"
	CodecSoftwareJPEG Codec = "software-jpeg"
)

type Selection struct {
	Requested Codec
	Selected  Codec
	Backend   string
	Hardware  bool
	Detail    string
}

func Select(requested Codec) (Selection, error) {
	if requested == "" {
		requested = CodecAuto
	}
	if requested != CodecSoftwareAV1 && requested != CodecHardwareAV1 && requested != CodecAuto && requested != CodecHardwareH264 && requested != CodecHardwareHEVC && requested != CodecSoftwareH264 && requested != CodecHardwareJPEG && requested != CodecSoftwareJPEG {
		return Selection{}, fmt.Errorf("不支援 codec %q", requested)
	}
	if requested == CodecSoftwareJPEG {
		return Selection{requested, CodecSoftwareJPEG, "Go image/jpeg", false, "明確指定軟體編碼"}, nil
	}
	if requested == CodecSoftwareAV1 {
		e, err := newSoftwareAV1Encoder()
		if err != nil {
			return Selection{}, err
		}
		e.Close()
		return Selection{requested, CodecSoftwareAV1, "FFmpeg / libaom AV1 CPU", false, "AV1 軟體編碼"}, nil
	}
	caps := detectCapabilities()
	if requested == CodecHardwareJPEG {
		return SelectJPEGEncoder(requested), nil
	}
	if requested == CodecSoftwareH264 {
		if caps.swH264 {
			return Selection{requested, CodecSoftwareH264, "software H.264", false, "軟體 H.264 encoder 可用"}, nil
		}
		return selectJPEGFallback(requested, "軟體 H.264 encoder 不可用"), nil
	}
	if requested == CodecHardwareAV1 {
		if caps.av1 {
			return Selection{requested, CodecHardwareAV1, caps.backend, true, caps.detail}, nil
		}
		fallback, err := Select(CodecHardwareHEVC)
		fallback.Requested = requested
		return fallback, err
	}
	if requested == CodecHardwareHEVC {
		if caps.hevc {
			return Selection{requested, CodecHardwareHEVC, caps.backend, true, caps.detail}, nil
		}
		if caps.h264 {
			return Selection{requested, CodecHardwareH264, caps.backend, true, "HEVC 硬體編碼不可用，退回硬體 H.264；" + caps.detail}, nil
		}
		if caps.swH264 {
			return Selection{requested, CodecSoftwareH264, "software H.264", false, "HEVC/H.264 硬體編碼不可用，退回軟體 H.264"}, nil
		}
		return selectJPEGFallback(requested, "HEVC/H.264 編碼不可用"), nil
	}
	if requested == CodecHardwareH264 {
		if caps.h264 {
			return Selection{requested, CodecHardwareH264, caps.backend, true, caps.detail}, nil
		}
		if caps.swH264 {
			return Selection{requested, CodecSoftwareH264, "software H.264", false, "H.264 硬體編碼不可用，退回軟體 H.264"}, nil
		}
		return selectJPEGFallback(requested, "H.264 編碼不可用"), nil
	}
	if caps.hevc {
		return Selection{requested, CodecHardwareHEVC, caps.backend, true, caps.detail}, nil
	}
	if caps.h264 {
		return Selection{requested, CodecHardwareH264, caps.backend, true, caps.detail}, nil
	}
	if caps.swH264 {
		return Selection{requested, CodecSoftwareH264, "software H.264", false, "軟體 H.264 encoder 可用"}, nil
	}
	return selectJPEGFallback(requested, "HEVC/H.264 encoder 不可用"), nil
}

// selectJPEGFallback keeps the requested mode for diagnostics while inserting
// the runtime-probed hardware JPEG tier before the guaranteed software path.
func selectJPEGFallback(requested Codec, reason string) Selection {
	jpegSelection := SelectJPEGEncoder(CodecAuto)
	jpegSelection.Requested = requested
	jpegSelection.Detail = reason + "；" + jpegSelection.Detail
	return jpegSelection
}

type capabilities struct {
	h264, hevc, av1, swH264 bool
	backend, detail         string
}

type DecoderSelection struct {
	Hardware bool
	Backend  string
	Codec    Codec
	Probed   bool
	Detail   string
}

func SelectDecoder() DecoderSelection {
	caps := detectCapabilities()
	if caps.h264 || caps.hevc {
		return DecoderSelection{Hardware: true, Backend: caps.backend, Codec: CodecHardwareH264, Probed: false, Detail: "硬體解碼候選已偵測；啟動時仍需 codec runtime 驗證"}
	}
	return DecoderSelection{Hardware: false, Backend: "software", Codec: CodecSoftwareJPEG, Probed: true, Detail: "未偵測到硬體解碼器，使用軟體解碼"}
}
