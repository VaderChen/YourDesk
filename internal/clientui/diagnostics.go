package clientui

import (
	"net/http"
	"strconv"
	"time"
	"yourdesk/internal/diagnostics"
)

// 呼叫時已持有 server.mu。取樣綁定子程序，重新連線不沿用舊報告。
type diagnosticRun struct {
	StartedAt int64                `json:"startedAt"`
	Samples   []diagnostics.Sample `json:"samples"`
}

func (s *server) connectionDiagnostics(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	var site *Site
	for i := range s.library.Sites {
		if s.library.Sites[i].ID == id {
			site = &s.library.Sites[i]
			break
		}
	}
	if site == nil {
		respond(w, 404, map[string]string{"error": "找不到站台"})
		return
	}
	p := s.children["viewer:"+id]
	if r.Method == "DELETE" {
		started, _ := strconv.ParseInt(r.URL.Query().Get("startedAt"), 10, 64)
		if p != nil && p.diagnosticConnection && p.diagnostic != nil && p.diagnostic.StartedAt == started {
			_ = p.cmd.Process.Kill()
		}
		respond(w, 200, map[string]bool{"ok": true})
		return
	}
	if p == nil || p.stage != "connected" || p.room != site.Room {
		respond(w, 200, map[string]any{"connected": false})
		return
	}
	if r.Method == "POST" {
		p.diagnostic = &diagnosticRun{StartedAt: time.Now().UnixMilli(), Samples: []diagnostics.Sample{}}
	}
	respond(w, 200, map[string]any{"connected": true, "run": p.diagnostic})
}
