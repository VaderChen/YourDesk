package clientui

import (
	"errors"
	"net/http"
	"time"
	"yourdesk/internal/authlog"
	"yourdesk/internal/security"
)

func (s *server) packetTest(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID     string `json:"id"`
		Action string `json:"action"`
		Token  string `json:"token"`
		Secret string `json:"secret"`
	}
	if err := decode(w, r, &in); err != nil {
		fail(w, err)
		return
	}
	key := "viewer:packet-" + in.ID
	if in.Action != "stop" && !authlog.IsEnabled() {
		fail(w, errors.New("請先開啟網路 Debug"))
		return
	}
	s.mu.Lock()
	p := s.children[key]
	if in.Action == "start" {
		if p != nil {
			s.mu.Unlock()
			fail(w, errors.New("此站台的封包測試正在執行"))
			return
		}
		var selected *Site
		for i := range s.library.Sites {
			if s.library.Sites[i].ID == in.ID {
				selected = &s.library.Sites[i]
				break
			}
		}
		if selected == nil {
			s.mu.Unlock()
			fail(w, errors.New("找不到站台"))
			return
		}
		secret := in.Secret
		if secret == "" {
			secret = s.remembered(selected.Signal, selected.Room)
		}
		if secret == "" {
			s.mu.Unlock()
			respond(w, 200, map[string]bool{"needsSecret": true})
			return
		}
		if _, err := security.DecodeSecret(secret); err != nil {
			s.mu.Unlock()
			fail(w, err)
			return
		}
		args := []string{"-signal", selected.Signal, "-room", selected.Room, "-name", selected.Name, "-secret-stdin", "-mcp-managed", "-mcp-hidden", "-mcp-paused"}
		if s.preferences.TailcatEnabled {
			args = append(args, "-transport", "tailcat")
		}
		if err := s.start("viewer", "packet-"+in.ID, s.viewerBinary(), args); err != nil {
			s.mu.Unlock()
			fail(w, err)
			return
		}
		p = s.children[key]
		p.room = selected.Room
		p.mcpVisible = false
		if err := sendProcessSecret(p, secret); err != nil {
			_ = p.cmd.Process.Kill()
			s.mu.Unlock()
			fail(w, err)
			return
		}
		token := p.terminalInstance
		s.mu.Unlock()
		// UI 消失仍會回收測試專用程序；不影響同站台原有連線。
		go func() {
			timer := time.NewTimer(150 * time.Second)
			defer timer.Stop()
			select {
			case <-p.done:
				return
			case <-timer.C:
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.children[key] == p {
				_ = p.cmd.Process.Kill()
			}
		}()
		respond(w, 200, map[string]any{"token": token})
		return
	}
	if p == nil || in.Token == "" || p.terminalInstance != in.Token {
		s.mu.Unlock()
		fail(w, errors.New("測試連線已結束"))
		return
	}
	if in.Action == "stop" {
		_ = p.cmd.Process.Kill()
		s.mu.Unlock()
		respond(w, 200, map[string]bool{"ok": true})
		return
	}
	if in.Action == "status" {
		stage := p.stage
		s.mu.Unlock()
		respond(w, 200, map[string]string{"stage": stage})
		return
	}
	valid := p.stage == "connected"
	s.mu.Unlock()
	if !valid {
		fail(w, errors.New("測試連線尚未就緒"))
		return
	}
	if in.Action != "batch" {
		fail(w, errors.New("不支援的測試操作"))
		return
	}
	out, err := s.callRemoteAgent(r.Context(), mcpAction{expectedProcess: p, Session: key, Action: "network.test"})
	if err != nil {
		fail(w, err)
		return
	}
	respond(w, 200, out.Result)
}

func (s *server) networkDebug(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		var in struct {
			Enabled bool `json:"enabled"`
		}
		if err := decode(w, r, &in); err != nil {
			fail(w, err)
			return
		}
		if err := authlog.SetDebug(in.Enabled); err != nil {
			fail(w, err)
			return
		}
	}
	respond(w, 200, map[string]bool{"enabled": authlog.IsEnabled()})
}
