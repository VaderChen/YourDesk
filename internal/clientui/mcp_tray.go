package clientui

import (
	"context"
	"encoding/json"
	"time"
	"yourdesk/internal/agentremote"
	"yourdesk/internal/prelogin"
)

type mcpConnectContextKey struct{}

func contextMCP(args []string) (string, bool) {
	for _, a := range args {
		if a == "-mcp-hidden" || a == "-mcp-managed" {
			return a, true
		}
	}
	return "", false
}
func (s *server) mcpSessionCount() (int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	visible := false
	for _, p := range s.children {
		if p.mcpOwned {
			n++
			visible = visible || p.mcpVisible
		}
	}
	return n, visible
}
func (s *server) showMCPWindows() {
	s.mu.Lock()
	defer s.mu.Unlock()
	visible := false
	for _, p := range s.children {
		visible = visible || (p.mcpOwned && p.mcpVisible)
	}
	for _, p := range s.children {
		if !p.mcpOwned || p.stdin == nil {
			continue
		}
		setMCPVisibility(p, !visible)
	}
}

type trayCallbacks struct {
	disconnectIncoming func()
	quit               func()
	showMCP            func()
}

// 呼叫端持有 server.mu，私有操作管道依序傳送。
func setMCPVisibility(p *process, visible bool) {
	action := "hide"
	if visible {
		action = "show"
	}
	if err := json.NewEncoder(p.stdin).Encode(map[string]any{"agent": agentremote.Request{ID: "tray-visibility", Action: action, Expires: time.Now().Add(10 * time.Second).UnixMilli()}}); err == nil {
		p.mcpVisible = visible
	}
}

func (s *server) incomingConnected() bool {
	if prelogin.Status().Enabled {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		connected, _ := prelogin.Incoming(ctx, false)
		return connected
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.children["host"]
	return p != nil && p.incomingConnected
}
func (s *server) disconnectIncoming() error {
	if prelogin.Status().Enabled {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := prelogin.Incoming(ctx, true)
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.children["host"]
	if p == nil || !p.incomingConnected || p.stdin == nil {
		return nil
	}
	return json.NewEncoder(p.stdin).Encode(map[string]bool{"disconnect": true})
}
func (s *server) stopIncomingFromTray() {
	if err := s.disconnectIncoming(); err != nil {
		s.mu.Lock()
		s.notice = "無法關閉遠端連線：" + err.Error()
		s.mu.Unlock()
	}
}
