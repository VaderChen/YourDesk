package clientui

import (
	"context"
	"errors"
	"net/http"
	"time"
	"yourdesk/internal/prelogin"
)

// 呼叫端持有 s.mu，與 Host 啟動／服務交接共用同一把鎖。
func (s *server) preloginState() prelogin.State {
	state := prelogin.Status()
	state.Busy = s.preloginBusy
	if s.preloginMessage != "" {
		state.Message = s.preloginMessage
		// 進度與一般說明不是錯誤；非忙碌時留下的操作訊息才代表失敗。
		if !s.preloginBusy {
			state.Error = s.preloginMessage
		}
	}
	return state
}
func (s *server) handlePrelogin(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path != "/api/prelogin" {
		return false
	}
	if r.Method == "GET" {
		s.mu.Lock()
		defer s.mu.Unlock()
		respond(w, 200, s.preloginState())
		return true
	}
	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return true
	}
	var request struct {
		Enabled bool `json:"enabled"`
	}
	if err := decode(w, r, &request); err != nil {
		fail(w, err)
		return true
	}
	s.mu.Lock()
	if s.preloginBusy {
		s.mu.Unlock()
		fail(w, errors.New("未登入服務正在變更，請稍候。"))
		return true
	}
	status := prelogin.Status()
	if !status.Supported {
		s.mu.Unlock()
		fail(w, errors.New(status.Message))
		return true
	}
	if status.Enabled == request.Enabled {
		s.mu.Unlock()
		respond(w, 200, map[string]bool{"ok": true})
		return true
	}
	s.preloginBusy = true
	s.preloginMessage = "正在變更系統服務，請完成管理員授權。"
	config := prelogin.Config{Tailcat: s.preferences.TailcatEnabled, Room: s.info["room"], Signal: s.options.Signal, Secret: s.options.Secret, Codec: s.preferences.Codec, CodecGoal: s.preferences.CodecGoal, Direct: s.preferences.DirectListen}
	host := s.children["host"]
	if host != nil {
		host.stdin.Close()
	}
	s.mu.Unlock()
	// 授權對話框可能超過 HTTP 逾時；用 APP 生命週期與狀態輪詢管理工作。
	go func() {
		ctx, cancel := context.WithTimeout(s.updater.ctx, 2*time.Minute)
		defer cancel()
		var err error
		if host != nil {
			select {
			case <-host.done:
			case <-ctx.Done():
				err = ctx.Err()
			}
		}
		if err == nil {
			err = prelogin.Configure(ctx, request.Enabled, config)
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		s.preloginBusy = false
		if err != nil {
			s.preloginMessage = err.Error()
		} else {
			s.preloginMessage = ""
		}
	}()
	respond(w, 202, map[string]bool{"pending": true})
	return true
}
