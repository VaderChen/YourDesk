// Package optimization 保存經本機偵測的策略；不根據首張延遲猜測 codec 排名。
package optimization

import (
	"runtime"
	"sync/atomic"
)

type Encoder struct {
	Codec  string `json:"codec"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Usable bool   `json:"usable"`
}

// Decoder.Mode 是路徑偏好；software 可代表硬解實測失敗，不保證軟解成功。
type Decoder struct {
	Codec  string `json:"codec"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Mode   string `json:"mode"`
}
type Policy struct {
	Version      int       `json:"version"`
	OS           string    `json:"os"`
	Arch         string    `json:"arch"`
	CompareBytes int       `json:"compareBytes"`
	Encoders     []Encoder `json:"encoders"`
	Decoders     []Decoder `json:"decoders,omitempty"`
}

var current atomic.Pointer[Policy]

// Apply 只接受同平台、受限記憶體及已知 codec 的本機策略。
func Apply(p Policy) bool {
	if p.Version != 1 || p.OS != runtime.GOOS || p.Arch != runtime.GOARCH || p.CompareBytes < 0 || p.CompareBytes > 64<<20 || len(p.Encoders) > 32 || len(p.Decoders) > 32 {
		return false
	}
	for _, e := range p.Encoders {
		if (e.Codec != "h264" && e.Codec != "hevc" && e.Codec != "av1") || e.Width < 1 || e.Height < 1 || e.Width > 8192 || e.Height > 8192 {
			return false
		}
	}
	for i, d := range p.Decoders {
		if (d.Codec != "h264" && d.Codec != "hevc" && d.Codec != "av1") || d.Width < 1 || d.Height < 1 || d.Width > 8192 || d.Height > 8192 || (d.Mode != "hardware" && d.Mode != "software" && d.Mode != "auto" && d.Mode != "unavailable") {
			return false
		}
		for _, previous := range p.Decoders[:i] {
			if previous.Codec == d.Codec && previous.Width == d.Width && previous.Height == d.Height {
				return false
			}
		}
	}
	p.Encoders = append([]Encoder(nil), p.Encoders...)
	p.Decoders = append([]Decoder(nil), p.Decoders...)
	current.Store(&p)
	return true
}
func Snapshot() Policy {
	if p := current.Load(); p != nil {
		copy := *p
		copy.Encoders = append([]Encoder(nil), p.Encoders...)
		copy.Decoders = append([]Decoder(nil), p.Decoders...)
		return copy
	}
	return Policy{}
}

// DecoderDecision 僅命中實際壓縮影格尺寸；沒有證據時維持平台自選。
func (p Policy) DecoderDecision(codec string, w, h int) string {
	for _, d := range p.Decoders {
		if d.Codec == codec && d.Width == w && d.Height == h {
			return d.Mode
		}
	}
	return ""
}

// EncoderDecision 僅命中實測尺寸；不將 1080p 支援外推為 4K 或其他尺寸。
func (p Policy) EncoderDecision(codec string, w, h int) (usable, known bool) {
	for _, e := range p.Encoders {
		if e.Codec == codec && e.Width == w && e.Height == h {
			return e.Usable, true
		}
	}
	return false, false
}
