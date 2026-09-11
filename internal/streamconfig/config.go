// Package streamconfig 統一預設組合、來源參數驗證與舊協定轉接。
package streamconfig

import (
	"fmt"
	"image"
	"math"
)

const Version = 1
const DefaultKeyframeInterval = 10
const MaxKeyframeInterval = 300

type Resolution struct {
	Mode          string `json:"mode"`
	ScalePercent  int    `json:"scalePercent,omitempty"`
	MaxWidth      int    `json:"maxWidth,omitempty"`
	MaxHeight     int    `json:"maxHeight,omitempty"`
	ViewportLimit bool   `json:"viewportLimit"`
}
type FPS struct {
	Mode              string `json:"mode"`
	Limit             int    `json:"limit,omitempty"`
	MultiplierPercent int    `json:"multiplierPercent,omitempty"`
}
type Rate struct {
	Mode      string `json:"mode"`
	TargetBps int    `json:"targetBps,omitempty"`
}
type Source struct {
	KeyframeInterval int        `json:"keyframeInterval,omitempty"`
	Resolution       Resolution `json:"resolution"`
	FPS              FPS        `json:"fps"`
	Rate             Rate       `json:"rate"`
	Quality          int        `json:"quality,omitempty"`
}
type Request struct {
	SchemaVersion int    `json:"schemaVersion"`
	SessionID     string `json:"sessionId"`
	Revision      uint64 `json:"revision"`
	Profile       string `json:"profile"`
	ViewWidth     int    `json:"viewWidth"`
	ViewHeight    int    `json:"viewHeight"`
	Source        Source `json:"source"`
}
type Capabilities struct {
	MaxKeyframeInterval int      `json:"maxKeyframeInterval,omitempty"`
	SchemaVersion       int      `json:"schemaVersion"`
	SessionID           string   `json:"sessionId"`
	ResolutionModes     []string `json:"resolutionModes"`
	FPSModes            []string `json:"fpsModes"`
	RateModes           []string `json:"rateModes"`
}
type Effective struct {
	KeyframeInterval int `json:"keyframeInterval,omitempty"`
	Width            int `json:"width"`
	Height           int `json:"height"`
	FPS              int `json:"fps"`
	Bitrate          int `json:"bitrate"`
	Quality          int `json:"quality"`
}
type Result struct {
	SessionID string    `json:"sessionId"`
	Revision  uint64    `json:"revision"`
	Accepted  bool      `json:"accepted"`
	Error     string    `json:"error,omitempty"`
	Effective Effective `json:"effective"`
}

func Advertise(session string) *Capabilities {
	return &Capabilities{MaxKeyframeInterval, Version, session, []string{"inherit", "native", "scale", "fit"}, []string{"inherit", "limit", "multiplier"}, []string{"inherit", "quality", "cbr"}}
}

// Compose 是本次增強的預設組合。FPS 與碼率不受放大演算法影響。
func Compose(profile string, w, h int, enhance bool) Request {
	r := Request{SchemaVersion: Version, Profile: profile, ViewWidth: w, ViewHeight: h, Source: Source{Resolution: Resolution{Mode: "inherit"}, FPS: FPS{Mode: "inherit"}, Rate: Rate{Mode: "inherit"}}}
	if enhance {
		r.Source.Resolution = Resolution{Mode: "scale", ScalePercent: 75}
	}
	return r
}

// ComposeLegacy 保留舊旗標的半解析度語意，不隨新版預設改變。
func ComposeLegacy(profile string, w, h int, enhance bool) Request {
	r := Compose(profile, w, h, enhance)
	if enhance {
		r.Source.Resolution.ScalePercent = 50
	}
	return r
}

func (r Request) Validate() error {
	if r.SchemaVersion != Version {
		return fmt.Errorf("不支援的串流設定版本")
	}
	if r.Profile != "fast" && r.Profile != "standard" && r.Profile != "high" {
		return fmt.Errorf("不支援的畫質模式")
	}
	if r.ViewWidth < 1 || r.ViewHeight < 1 || r.ViewWidth > 8192 || r.ViewHeight > 8192 {
		return fmt.Errorf("視窗尺寸超出範圍")
	}
	s := r.Source
	if s.KeyframeInterval < 0 || s.KeyframeInterval > MaxKeyframeInterval {
		return fmt.Errorf("GOP 必須為 1 至 300 張；0 沿用相容設定")
	}
	switch s.Resolution.Mode {
	case "inherit", "native":
		if s.Resolution.ScalePercent != 0 || s.Resolution.MaxWidth != 0 || s.Resolution.MaxHeight != 0 || s.Resolution.ViewportLimit {
			return fmt.Errorf("解析度模式與參數不相容")
		}
	case "scale":
		if s.Resolution.ScalePercent < 25 || s.Resolution.ScalePercent > 100 || s.Resolution.MaxWidth != 0 || s.Resolution.MaxHeight != 0 {
			return fmt.Errorf("縮放比例超出範圍")
		}
	case "fit":
		if s.Resolution.MaxWidth < 1 || s.Resolution.MaxHeight < 1 || s.Resolution.MaxWidth > 8192 || s.Resolution.MaxHeight > 8192 || s.Resolution.ScalePercent != 0 {
			return fmt.Errorf("縮圖尺寸超出範圍")
		}
	default:
		return fmt.Errorf("不支援的解析度模式")
	}
	switch s.FPS.Mode {
	case "inherit":
		if s.FPS.Limit != 0 || s.FPS.MultiplierPercent != 0 {
			return fmt.Errorf("FPS 模式與參數不相容")
		}
	case "limit":
		if s.FPS.Limit < 1 || s.FPS.Limit > 240 || s.FPS.MultiplierPercent != 0 {
			return fmt.Errorf("FPS 上限超出範圍")
		}
	case "multiplier":
		if s.FPS.MultiplierPercent < 25 || s.FPS.MultiplierPercent > 400 || s.FPS.Limit != 0 {
			return fmt.Errorf("FPS 倍率超出範圍")
		}
	default:
		return fmt.Errorf("不支援的 FPS 模式")
	}
	switch s.Rate.Mode {
	case "inherit", "quality":
		if s.Rate.TargetBps != 0 {
			return fmt.Errorf("碼率模式與參數不相容")
		}
	case "cbr":
		if s.Rate.TargetBps < 64000 || s.Rate.TargetBps > 100000000 {
			return fmt.Errorf("碼率超出範圍")
		}
	default:
		return fmt.Errorf("不支援的碼率模式")
	}
	if s.Quality < 0 || s.Quality > 100 {
		return fmt.Errorf("編碼品質超出範圍")
	}
	return nil
}
func (r Request) Dimensions(size image.Point) image.Point {
	w, h := size.X, size.Y
	switch r.Source.Resolution.Mode {
	case "inherit":
		if r.Profile == "fast" {
			w = min(r.ViewWidth, max(1, w/2))
			h = min(r.ViewHeight, max(1, h/2))
		}
	case "scale":
		w = max(1, w*r.Source.Resolution.ScalePercent/100)
		h = max(1, h*r.Source.Resolution.ScalePercent/100)
	case "fit":
		w = min(w, r.Source.Resolution.MaxWidth)
		h = min(h, r.Source.Resolution.MaxHeight)
	}
	if r.Source.Resolution.ViewportLimit {
		w = min(w, r.ViewWidth)
		h = min(h, r.ViewHeight)
	}
	return image.Pt(w, h)
}
func (r Request) FrameRate(base int) int {
	n := max(1, base)
	if r.Profile == "fast" && n < 30 {
		n = min(30, n+(n+1)/2)
	}
	switch r.Source.FPS.Mode {
	case "limit":
		n = r.Source.FPS.Limit
	case "multiplier":
		n = max(1, min(240, n*r.Source.FPS.MultiplierPercent/100))
	}
	return n
}
func (r Request) EncodeQuality(base int) int {
	q := max(1, min(100, base))
	if r.Profile == "fast" {
		q = min(q, 50)
	} else if r.Profile == "high" {
		q = max(q, 95)
	}
	if r.Source.Quality > 0 {
		q = r.Source.Quality
	}
	return q
}
func (r Request) Bitrate(original image.Point, baseFPS int) int {
	switch r.Source.Rate.Mode {
	case "cbr":
		return r.Source.Rate.TargetBps
	case "quality":
		return 0
	}
	if r.Profile != "fast" {
		return 0
	}
	// 沿用原模式的預算，與額外解析度覆寫解耦。
	w := min(r.ViewWidth, max(1, original.X/2))
	h := min(r.ViewHeight, max(1, original.Y/2))
	if r.Source.Resolution.Mode == "inherit" {
		scale := math.Min(1, math.Min(float64(w)/float64(original.X), float64(h)/float64(original.Y)))
		w, h = max(1, int(float64(original.X)*scale)), max(1, int(float64(original.Y)*scale))
	}
	return min(4000000, max(256000, w*h*baseFPS/8))
}
