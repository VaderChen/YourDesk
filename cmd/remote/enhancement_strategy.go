package main

import (
	"slices"
	"yourdesk/internal/streamconfig"
)

type enhancementStrategy struct {
	Mode        string
	BitrateMbps int
}

func normalizeStrategy(mode string, mbps int) *enhancementStrategy {
	if mode != "smooth" && mode != "traffic" {
		mode = "quality"
	}
	if mbps < 1 || mbps > 100 {
		mbps = 12
	}
	return &enhancementStrategy{mode, mbps}
}
func currentStrategy() *enhancementStrategy {
	if value := viewerStrategy.Load(); value != nil {
		return value
	}
	return normalizeStrategy("quality", 12)
}
func strategySupported(c *streamconfig.Capabilities, mode string) bool {
	if mode == "quality" {
		return true
	}
	return c != nil && slices.Contains(c.RateModes, "cbr") && (mode != "smooth" || slices.Contains(c.FPSModes, "multiplier"))
}

// 策略僅組合現有協定欄位，不增加來源端功能旗標。
func applyEnhancementStrategy(r *streamconfig.Request, c *streamconfig.Capabilities, enabled bool) {
	s := currentStrategy()
	if !enabled || s.Mode == "quality" || !strategySupported(c, s.Mode) {
		return
	}
	r.Source.Rate = streamconfig.Rate{Mode: "cbr", TargetBps: s.BitrateMbps * 1000000}
	if s.Mode == "traffic" {
		r.Source.Rate.TargetBps /= 2
	} else {
		r.Source.FPS = streamconfig.FPS{Mode: "multiplier", MultiplierPercent: 200}
	}
}
