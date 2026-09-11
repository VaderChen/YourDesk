package clientui

import (
	"errors"
	"math"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// 使用目前 遠端顯示 的新樣本，不把本機 smoke 的固定數字套到其他電腦。
// 此端點與偏好儲存共用 server.mu，只更新 FPS/GOP，其餘偏好保留。
func (s *server) autoStream(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Mode      string `json:"mode"`
		ID        string `json:"id"`
		StartedAt int64  `json:"startedAt"`
		Apply     bool   `json:"apply"`
	}
	if err := decode(w, r, &request); err != nil {
		fail(w, err)
		return
	}
	if request.Mode != "latency" && request.Mode != "traffic" {
		fail(w, errors.New("不支援的自動配置"))
		return
	}
	var selected *process
	for _, p := range s.children {
		if p.kind != "viewer" || p.stage != "connected" || request.ID != "" && p.siteID != request.ID {
			continue
		}
		if selected != nil {
			fail(w, errors.New("有多個遠端連線，請先對要配置的站台按測速，再回來配置。"))
			return
		}
		selected = p
	}
	if selected == nil {
		fail(w, errors.New("請先連線到遠端，再使用自動配置。"))
		return
	}
	if request.StartedAt == 0 {
		selected.diagnostic = &diagnosticRun{StartedAt: time.Now().UnixMilli()}
		respond(w, 200, map[string]any{"pending": true, "id": selected.siteID, "startedAt": selected.diagnostic.StartedAt})
		return
	}
	d := selected.diagnostic
	if d == nil || d.StartedAt != request.StartedAt || time.Now().UnixMilli()-d.StartedAt > 30000 {
		fail(w, errors.New("連線已改變，請重新分析。"))
		return
	}
	seconds, activeSeconds, frames := 0.0, 0.0, 0.0
	var gaps, received uint64
	jpeg := false
	for _, sample := range d.Samples {
		rate := sample.SourceFPS
		if sample.Background {
			rate = sample.DecodedPerSec
		}
		if math.IsNaN(sample.Seconds) || math.IsInf(sample.Seconds, 0) || sample.Seconds <= 0 || sample.Seconds > 10 || math.IsNaN(rate) || math.IsInf(rate, 0) || rate < 0 {
			continue
		}
		isJPEG := strings.HasPrefix(sample.Codec, "JPEG")
		if sample.Background && isJPEG {
			continue
		}
		seconds += sample.Seconds
		received += sample.Received
		gaps += sample.Gaps + sample.Errors
		jpeg = jpeg || isJPEG
		if rate >= 5 && sample.Received > 0 {
			activeSeconds += sample.Seconds
			frames += rate * sample.Seconds
		}
	}
	if seconds < 9.5 {
		respond(w, 200, map[string]any{"pending": true, "id": selected.siteID, "startedAt": d.StartedAt, "sampleSeconds": seconds})
		return
	}
	if received < 10 || activeSeconds < 5 {
		fail(w, errors.New("畫面變化太少，請持續拖動視窗或捲動後再試。"))
		return
	}
	measuredFPS := frames / activeSeconds
	prefs := s.preferences
	oldFPS, oldGOP := prefs.SourceFPSLimit, prefs.KeyframeInterval
	if oldFPS < 5 || oldFPS > 60 {
		oldFPS = 20
	}
	if oldGOP < 1 || oldGOP > 300 {
		oldGOP = 10
	}
	if request.Mode == "latency" {
		// 僅降低超出已觀察能力的要求；被既有上限限制時，不假定更高 FPS 可達成。
		prefs.SourceFPSLimit = min(oldFPS, max(5, int(math.Floor(measuredFPS))))
		if !jpeg {
			prefs.KeyframeInterval = min(oldGOP, max(1, prefs.SourceFPSLimit/2))
		}
	} else {
		prefs.SourceFPSLimit = min(oldFPS, min(15, max(5, int(math.Floor(measuredFPS*0.6)))))
		if !jpeg {
			prefs.KeyframeInterval = max(oldGOP, prefs.SourceFPSLimit*2)
		} // 至少兩秒一張完整畫面，保留原本更長的間隔。
	}
	if !jpeg && gaps > 0 {
		prefs.KeyframeInterval = min(prefs.KeyframeInterval, max(1, prefs.SourceFPSLimit/2))
	}
	if err := prefs.validate(); err != nil {
		fail(w, err)
		return
	}
	if !request.Apply {
		respond(w, 200, map[string]any{"pending": false, "id": selected.siteID, "startedAt": d.StartedAt, "preferences": prefs, "measuredFPS": measuredFPS, "sampleSeconds": seconds, "activeSeconds": activeSeconds})
		return
	}
	if err := saveJSON(filepath.Join(filepath.Dir(s.configPath), "preferences.json"), prefs); err != nil {
		fail(w, err)
		return
	}
	s.preferences = prefs
	respond(w, 200, map[string]any{"pending": false, "preferences": prefs, "id": selected.siteID, "measuredFPS": measuredFPS, "sampleSeconds": seconds, "activeSeconds": activeSeconds})
}
