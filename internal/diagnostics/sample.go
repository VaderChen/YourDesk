// Package diagnostics 定義本機 遠端顯示 回報的實際連線樣本，不推估頻寬上限。
package diagnostics

type Sample struct {
	RemoteVersion  string   `json:"remoteVersion,omitempty"`
	Background     bool     `json:"background,omitempty"`
	DecodedPerSec  float64  `json:"decodedPerSec,omitempty"`
	StartedAt      int64    `json:"startedAt"`
	Seconds        float64  `json:"seconds"`
	RTTMS          *float64 `json:"rttMs"`
	ReceiveMbps    float64  `json:"receiveMbps"`
	SourceFPS      float64  `json:"sourceFps"`
	RenderFPS      float64  `json:"renderFps"`
	ReceivedPerSec float64  `json:"receivedPerSec"`
	CallbackMS     float64  `json:"callbackMs"`
	FPSLimit       int      `json:"fpsLimit"`
	Codec          string   `json:"codec"`
	Gaps           uint64   `json:"gaps"`
	Errors         uint64   `json:"errors"`
	Received       uint64   `json:"received"`
}
