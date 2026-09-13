package signaling

import (
	"context"
	"net/http"
	"time"
)

const heartbeatProtocolHeader = "YourDesk-Heartbeat-Protocol"
const heartbeatV2 = "heartbeat-v2"

type heartbeatSettings struct {
	Protocol          string
	Interval, Timeout time.Duration
}

// v2 的時序是版本契約；接受不完整或未知回覆時仍用 v1 較密集的心跳。
func acceptedHeartbeat(headers http.Header, transport string) heartbeatSettings {
	fallback := heartbeatSettings{"legacy-v1", 60 * time.Second, 10 * time.Second}
	timeout := "10"
	if transport == "https" {
		fallback.Interval = 15 * time.Second
		fallback.Timeout = 45 * time.Second
		timeout = "180"
	}
	if headers.Get(heartbeatProtocolHeader) != heartbeatV2 || headers.Get("YourDesk-Heartbeat-Interval") != "60" || headers.Get("YourDesk-Heartbeat-Timeout") != timeout {
		return fallback
	}
	fallback.Protocol = heartbeatV2
	fallback.Interval = 60 * time.Second
	if transport == "https" {
		fallback.Timeout = 180 * time.Second
	}
	return fallback
}

func (h *httpConnection) runHeartbeat(ctx context.Context) {
	ticker := time.NewTicker(h.heartbeat.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pingCtx, end := context.WithTimeout(ctx, 5*time.Second)
			r, err := h.request(pingCtx, "POST", "/signal/heartbeat", nil, 0)
			terminal := false
			if err == nil {
				terminal = r.StatusCode == http.StatusGone || r.StatusCode == http.StatusUnauthorized
				r.Body.Close()
			}
			end()
			// 暫時網路失敗留待重試；Server 明確回收 session 時取消讀寫，讓上層重連。
			if terminal {
				h.cancel()
				return
			}
		}
	}
}
