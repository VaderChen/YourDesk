package clientui

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
)

func (s *server) terminalAction(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Session  string          `json:"session"`
		Instance string          `json:"instance"`
		Action   string          `json:"action"`
		Params   json.RawMessage `json:"params"`
	}
	if err := decode(w, r, &in); err != nil {
		fail(w, err)
		return
	}
	switch in.Action {
	case "open", "read", "write", "resize", "close", "disconnect":
	default:
		fail(w, errors.New("不支援的終端機操作"))
		return
	}
	s.mu.Lock()
	p := s.children[in.Session]
	valid := p != nil && p.terminalConnection && in.Instance != "" && p.terminalInstance == in.Instance
	if valid && in.Action == "disconnect" {
		_ = p.cmd.Process.Kill()
		s.mu.Unlock()
		respond(w, 200, map[string]bool{"ok": true})
		return
	}
	s.mu.Unlock()
	if !valid {
		fail(w, errors.New("終端機尚未連線"))
		return
	}
	out, err := s.callRemoteAgent(r.Context(), mcpAction{expectedProcess: p, Session: in.Session, Action: "terminal." + in.Action, Params: in.Params})
	if err != nil {
		fail(w, err)
		return
	}
	respond(w, 200, out.Result)
}

func (s *server) openTerminalWindow(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Session string `json:"session"`
	}
	if err := decode(w, r, &in); err != nil {
		fail(w, err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	remote := s.children[in.Session]
	if remote == nil || !remote.terminalConnection || remote.stage != "connected" {
		fail(w, errors.New("終端機尚未連線"))
		return
	}
	if remote.mcpOwned {
		fail(w, errors.New("此命令列由 MCP 使用，請另建連線開啟視窗"))
		return
	}
	windowID := remote.siteID + ":" + remote.terminalInstance
	key := "terminal-window:" + windowID
	if s.children[key] != nil {
		respond(w, 200, map[string]bool{"ok": true})
		return
	}
	title := "YourDesk"
	for _, site := range s.library.Sites {
		if site.ID == remote.siteID {
			title = site.Name
			break
		}
	}
	fragment := url.Values{"token": {s.token}, "session": {in.Session}, "name": {title}, "language": {s.preferences.Language}, "instance": {remote.terminalInstance}}
	address := s.origin + "/terminal.html#" + fragment.Encode()
	if err := s.start("terminal-window", windowID, s.executable, []string{"--terminal-window"}); err != nil {
		fail(w, err)
		return
	}
	window := s.children[key]
	window.terminalOwner = remote
	if err := json.NewEncoder(window.stdin).Encode(map[string]string{"url": address, "title": title + " · YourDesk Terminal"}); err != nil {
		_ = window.cmd.Process.Kill()
		fail(w, err)
		return
	}
	respond(w, 200, map[string]bool{"ok": true})
}
