package main

import (
	"slices"
	"sync/atomic"
	"time"
	"yourdesk/internal/p2p"
	"yourdesk/internal/streamconfig"
)

// sourceFPSLimit 是明確指定的 CLI 來源串流上限；未指定時使用介面偏好。
var sourceFPSLimit = 20
var sourceFPSOverride bool

// 偵測上限僅作用於此遠端視窗，逾時自動恢復，避免取消或主程式退出後殘留。
var fpsProbeUntil atomic.Int64

func applySourceFPSLimit(r *streamconfig.Request, c *streamconfig.Capabilities) {
	limits := streamingLimits{FPS: 20, BitrateMbps: 12}
	if p := viewerStreamingLimits.Load(); p != nil {
		limits = *p
	}
	if sourceFPSOverride {
		limits.FPS = sourceFPSLimit
	}
	if time.Now().UnixMilli() < fpsProbeUntil.Load() {
		limits.FPS = 30
	}
	if c == nil {
		return
	}
	if limits.FPS > 0 && slices.Contains(c.FPSModes, "limit") {
		r.Source.FPS = streamconfig.FPS{Mode: "limit", Limit: limits.FPS}
	}
	// 沿用 cbr 目標碼率協定；策略已有更低目標時保留較低值。
	if limits.BitrateMbps > 0 && slices.Contains(c.RateModes, "cbr") {
		target := limits.BitrateMbps * 1000000
		if r.Source.Rate.Mode == "cbr" && r.Source.Rate.TargetBps > 0 {
			target = min(target, r.Source.Rate.TargetBps)
		}
		r.Source.Rate = streamconfig.Rate{Mode: "cbr", TargetBps: target}
	}
}

func applyKeyframeInterval(r *streamconfig.Request, c *streamconfig.Capabilities) {
	n := int(viewerKeyframeInterval.Load())
	if n < 1 {
		n = streamconfig.DefaultKeyframeInterval
	}
	if c != nil && c.MaxKeyframeInterval >= n {
		r.Source.KeyframeInterval = n
	}
}

// 使用遠端畫面的可用實體像素，扣除工具列；連續調整視窗時等待尺寸穩定。
func (g *game) updateQuality() {
	if nativeFullscreenTransitioning() {
		g.qualityChangedAt = time.Now()
		return
	}
	if g.peer == nil || g.finalWidth < 1 || g.finalHeight <= g.toolbarHeight() {
		return
	}
	size := [2]int{min(8192, g.finalWidth), min(8192, g.finalHeight-g.toolbarHeight())}
	now := time.Now()
	if size != g.qualitySize {
		g.qualitySize = size
		g.qualityChangedAt = now
	}
	if g.quality == g.qualitySent {
		if g.quality == 0 && now.Sub(g.qualityChangedAt) < 200*time.Millisecond {
			return
		}
		if now.Sub(g.qualitySentAt) < time.Second {
			return
		}
	}
	profiles := []string{"fast", "standard", "high"}
	if capabilities := g.streamCapabilities.Load(); capabilities != nil {
		request := streamconfig.Compose(profiles[g.quality], size[0], size[1], g.enhancementEnabled)
		applyEnhancementStrategy(&request, capabilities, g.enhancementEnabled)
		applySourceFPSLimit(&request, capabilities)
		applyKeyframeInterval(&request, capabilities)
		request.SessionID = capabilities.SessionID
		request.Revision = g.streamRequest.Revision
		if request != g.streamRequest {
			g.streamRevision++
			request.Revision = g.streamRevision
		}
		if err := g.peer.SendControl(p2p.Control{Type: "stream-config", StreamConfig: &request}); err == nil {
			g.streamRequest = request
			g.qualitySent = g.quality
			g.qualitySentAt = now
		}
		return
	}
	if err := g.peer.SendControl(p2p.Control{Type: "quality-profile", Profile: profiles[g.quality], ImageEnhancement: g.enhancementEnabled, ViewWidth: size[0], ViewHeight: size[1]}); err == nil {
		g.qualitySent = g.quality
		g.qualitySentAt = now
	}
}
