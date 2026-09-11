package main

import (
	"time"
	"yourdesk/internal/streamconfig"
	"yourdesk/internal/superres"
)

// 實際來源回報與本機 GPU 處理分開判斷，避免只勾選設定就亮綠燈。
type enhancementDisplayStatus struct {
	Interpolation string `json:"interpolation"`
	Strategy      string `json:"strategy"`
	StrategyState string `json:"strategyState"`
	Algorithm     string `json:"algorithm"`
	Detail        string `json:"detail"`
	Active        bool   `json:"active"`
	Reason        string `json:"reason"`
	SourceWidth   int    `json:"sourceWidth"`
	SourceHeight  int    `json:"sourceHeight"`
	StreamWidth   int    `json:"streamWidth"`
	StreamHeight  int    `json:"streamHeight"`
	RenderWidth   int    `json:"renderWidth"`
	RenderHeight  int    `json:"renderHeight"`
	Bitrate       int    `json:"bitrate"`
}

func (g *game) publishEnhancementStatus(applied bool, width, height int) {
	status := enhancementDisplayStatus{RenderWidth: width, RenderHeight: height}
	status.Interpolation = g.interpolation.status
	status.Algorithm = "FSR 1 · EASU + RCAS"
	if g.superResolution > 0 {
		if g.mlApplied {
			status.Algorithm = "Core ML · " + superres.ModelName(int(g.superResolution)-1)
			status.Detail = "Core ML 自動分配運算裝置"
		} else {
			status.Detail = "Core ML 未就緒，暫用 FSR 1"
			if g.ml.reason != "" {
				status.Detail = g.ml.reason
			}
		}
	}
	report := g.remoteEnhancement.Load()
	if report != nil {
		status.SourceWidth, status.SourceHeight = report.SourceWidth, report.SourceHeight
		status.StreamWidth, status.StreamHeight = report.StreamWidth, report.StreamHeight
		status.Bitrate = report.Bitrate
	}
	switch {
	case !viewerImageEnhancement.Load():
		status.Reason = "尚未啟用"
	case g.enhancer == nil:
		status.Reason = "GPU 著色器無法使用"
	case !g.enhancementSupported.Load():
		status.Reason = "遠端尚未公告支援，請確認版本"
	case g.streamCapabilities.Load() != nil && (g.streamRequest.Revision == 0 || report == nil || report.ConfigRevision != g.streamRequest.Revision):
		status.Reason = "等待遠端套用增強"
		if result := g.streamResult.Load(); result != nil && result.Revision == g.streamRequest.Revision && result.SessionID == g.streamRequest.SessionID && !result.Accepted {
			status.Reason = "遠端拒絕串流設定"
			status.Detail = result.Error
		}
	case report == nil:
		status.Reason = "等待遠端回報增強狀態"
	case !report.Enabled:
		status.Reason = "等待遠端套用增強"
	case width < 1 || height < 1:
		status.Reason = "等待影格"
	case applied:
		status.Active = true
		status.Reason = "畫面增強已生效"
	case g.texture != nil && (width < g.texture.Bounds().Dx() || height < g.texture.Bounds().Dy()):
		status.Active = true
		status.Reason = "已降傳輸尺寸，目前視窗不需放大"
	default:
		status.Reason = "等待視窗尺寸穩定"
	}
	if result := g.streamResult.Load(); result != nil && result.Accepted && result.Revision == g.streamRequest.Revision && result.SessionID == g.streamRequest.SessionID && result.Error != "" {
		status.Detail = result.Error
	}
	if g.enhancementEnabled {
		strategy := currentStrategy()
		if strategy.Mode != "quality" && !strategySupported(g.streamCapabilities.Load(), strategy.Mode) {
			status.Detail = "遠端不支援此策略，維持原畫質策略"
		}
	}
	strategy := currentStrategy()
	status.Strategy = map[string]string{"quality": "自動調整", "smooth": "畫面流暢優先", "traffic": "網路負載優先"}[strategy.Mode]
	switch {
	case !g.enhancementEnabled:
		status.StrategyState = "尚未啟用"
	case !strategySupported(g.streamCapabilities.Load(), strategy.Mode):
		status.Strategy = "自動調整"
		status.StrategyState = "相容模式"
	case g.streamCapabilities.Load() != nil:
		expected := streamconfig.Compose(g.streamRequest.Profile, g.streamRequest.ViewWidth, g.streamRequest.ViewHeight, true)
		applyEnhancementStrategy(&expected, g.streamCapabilities.Load(), true)
		applySourceFPSLimit(&expected, g.streamCapabilities.Load())
		applyKeyframeInterval(&expected, g.streamCapabilities.Load())
		result := g.streamResult.Load()
		if expected.Source == g.streamRequest.Source && result != nil && result.Revision == g.streamRequest.Revision && result.SessionID == g.streamRequest.SessionID && !result.Accepted {
			status.StrategyState = "未套用"
		} else if expected.Source != g.streamRequest.Source || report == nil || report.ConfigRevision != g.streamRequest.Revision || result == nil || result.Revision != g.streamRequest.Revision || result.SessionID != g.streamRequest.SessionID {
			status.StrategyState = "等待套用"
		} else if !result.Accepted {
			status.StrategyState = "未套用"
		} else if result.Error != "" {
			status.StrategyState = "已回退"
		}
	case report == nil || !report.Enabled:
		status.StrategyState = "等待套用"
	}
	if g.enhancementEnabled && g.streamCapabilities.Load() == nil {
		status.Detail = "舊版協定僅支援半解析度，75% 需要新串流協定"
	}
	// 圖示代表整體增強狀態；短暫換幀／重新協商不閃燈，詳細原因仍即時更新。
	now := time.Now()
	if status.Active {
		g.enhancementIndicatorUntil = now.Add(750 * time.Millisecond)
	} else {
		transient := status.Reason == "等待視窗尺寸穩定" || status.Reason == "等待遠端套用增強" || status.Reason == "等待影格"
		if transient && now.Before(g.enhancementIndicatorUntil) {
			status.Active = true
		} else {
			g.enhancementIndicatorUntil = time.Time{}
		}
	}
	if status != g.lastEnhancementStatus {
		g.lastEnhancementStatus = status
		nativeSetEnhancementStatus(status)
	}
}
