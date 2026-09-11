package main

import (
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
)

//go:embed web/translations.json
var viewerTranslations string
var viewerCloseWindowOnDisconnect atomic.Bool
var viewerFitWindow atomic.Bool
var viewerDisableKeyMapping atomic.Bool
var viewerImageEnhancement atomic.Bool
var viewerStrategy atomic.Pointer[enhancementStrategy]
var viewerKeyframeInterval atomic.Int32

type streamingLimits struct {
	FPS         int
	BitrateMbps int
}

var viewerStreamingLimits atomic.Pointer[streamingLimits]

// 與主畫面共用語言偏好；背景重新讀取，已開啟的 遠端顯示 也會更新。
func refreshViewerLanguage(previous string) string {
	language := "auto"
	if dir, err := os.UserConfigDir(); err == nil {
		if data, err := os.ReadFile(filepath.Join(dir, "YourDesk", "preferences.json")); err == nil {
			var preferences struct {
				CloseWindowOnDisconnect bool   `json:"closeWindowOnDisconnect"`
				FitWindow               bool   `json:"fitWindow"`
				SourceFPSLimit          *int   `json:"sourceFPSLimit"`
				BitrateLimitMbps        int    `json:"bitrateLimitMbps"`
				EnhancementStrategy     string `json:"enhancementStrategy"`
				EnhancementBitrateMbps  int    `json:"enhancementBitrateMbps"`
				KeyframeInterval        int    `json:"keyframeInterval"`
				InterpolationMethod     string `json:"interpolationMethod"`
				Interpolation           bool   `json:"interpolation"`
				CoreMLModel             string `json:"coreMLModel"`
				SuperResolution         string `json:"superResolution"`
				ImageEnhancement        bool   `json:"imageEnhancement"`
				Language                string `json:"language"`
				DisableKeyMapping       bool   `json:"disableKeyMapping"`
			}
			if json.Unmarshal(data, &preferences) == nil {
				fps := 20
				if preferences.SourceFPSLimit != nil && *preferences.SourceFPSLimit >= 5 && *preferences.SourceFPSLimit <= 60 {
					fps = *preferences.SourceFPSLimit
				}
				mbps := preferences.BitrateLimitMbps
				if mbps < 1 || mbps > 64 {
					mbps = 12
				}
				viewerStreamingLimits.Store(&streamingLimits{fps, mbps})
				viewerCloseWindowOnDisconnect.Store(preferences.CloseWindowOnDisconnect)
				viewerFitWindow.Store(preferences.FitWindow)
				viewerDisableKeyMapping.Store(preferences.DisableKeyMapping)
				viewerImageEnhancement.Store(preferences.ImageEnhancement)
				viewerInterpolation.Store(preferences.Interpolation)
				switch preferences.InterpolationMethod {
				case "apple":
					viewerInterpolationMethod.Store(1)
				case "rife":
					viewerInterpolationMethod.Store(2)
				default:
					viewerInterpolationMethod.Store(0)
				}
				gop := preferences.KeyframeInterval
				if gop < 1 || gop > 300 {
					gop = 10
				}
				viewerKeyframeInterval.Store(int32(gop))
				viewerStrategy.Store(normalizeStrategy(preferences.EnhancementStrategy, preferences.EnhancementBitrateMbps))
				if preferences.SuperResolution == "coreml" {
					if preferences.CoreMLModel == "sesr-m5" {
						viewerSuperResolution.Store(2)
					} else {
						viewerSuperResolution.Store(1)
					}
				} else {
					viewerSuperResolution.Store(0)
				}
				switch preferences.Language {
				case "auto", "zh-Hant", "en", "ja", "ko":
					language = preferences.Language
				}
			}
		}
	}
	if language != previous {
		nativeSetLanguage(viewerTranslations, language)
	}
	return language
}
