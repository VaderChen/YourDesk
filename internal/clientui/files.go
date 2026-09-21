package clientui

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	"yourdesk/internal/agentremote"
)

func (s *server) filesAction(w http.ResponseWriter, r *http.Request) {
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
	case "status", "list", "stat", "read", "begin", "resume", "write", "commit", "cancel", "mkdir", "remove":
	default:
		fail(w, errors.New("不支援的檔案操作"))
		return
	}
	s.mu.Lock()
	p := s.children[in.Session]
	valid := p != nil && p.fileConnection && !p.mcpOwned && in.Instance != "" && p.terminalInstance == in.Instance
	connected := valid && p.stage == "connected"
	s.mu.Unlock()
	if in.Action == "status" {
		respond(w, 200, map[string]bool{"connected": connected})
		return
	}
	if !connected {
		filesSessionUnavailable(w)
		return
	}
	out, err := s.callRemoteAgent(r.Context(), mcpAction{expectedProcess: p, Session: in.Session, Action: "xfer." + in.Action, Params: in.Params})
	if err != nil {
		// A disconnect between admission and the response is resumable even if
		// this was the download's first stat, before any metadata was received.
		s.mu.Lock()
		stillConnected := s.children[in.Session] == p && p.stage == "connected"
		s.mu.Unlock()
		select {
		case <-p.done:
			stillConnected = false
		default:
		}
		if !stillConnected {
			filesSessionUnavailable(w)
			return
		}
		if out.Code == agentremote.CodeRequestInterrupted {
			respond(w, http.StatusBadRequest, map[string]string{"code": "files_request_interrupted", "error": "檔案操作暫時中斷或對端忙碌；請確認連線後繼續"})
			return
		}
		fail(w, err)
		return
	}
	respond(w, 200, out.Result)
}

func filesSessionUnavailable(w http.ResponseWriter) {
	respond(w, http.StatusBadRequest, map[string]string{"code": "files_session_unavailable", "error": "檔案傳輸尚未連線或工作階段已更換；請重新連線後繼續"})
}

func (s *server) openFilesWindow(w http.ResponseWriter, r *http.Request) {
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
	if remote == nil || !remote.fileConnection || remote.stage != "connected" || remote.mcpOwned {
		fail(w, errors.New("檔案傳輸尚未連線"))
		return
	}
	windowID := remote.siteID + ":" + remote.terminalInstance
	key := "files-window:" + windowID
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
	fragment := url.Values{"token": {s.token}, "session": {in.Session}, "instance": {remote.terminalInstance}, "name": {title}, "language": {s.preferences.Language}}
	address := s.origin + "/files.html#" + fragment.Encode()
	// 新的 Peer 已由主視窗完成認證。只重用同站台、同配對目標的舊視窗；
	// 不把舊 File／暫存 token 帶到使用者後來改成的另一台電腦。
	for _, window := range s.children {
		if !canRebindFilesWindow(window, remote) {
			continue
		}
		if window.fileOwner == remote {
			respond(w, 200, map[string]bool{"ok": true, "reused": true})
			return
		}
		if err := queueFilesWindowSession(window, remote, address, title); err != nil {
			fail(w, err)
			return
		}
		respond(w, 200, map[string]bool{"ok": true, "reused": true})
		return
	}
	if err := s.start("files-window", windowID, s.executable, []string{"--files-window"}); err != nil {
		fail(w, err)
		return
	}
	window := s.children[key]
	window.fileOwner = remote
	if err := json.NewEncoder(window.stdin).Encode(map[string]string{"url": address, "title": title + " · YourDesk Files"}); err != nil {
		_ = window.cmd.Process.Kill()
		fail(w, err)
		return
	}
	respond(w, 200, map[string]bool{"ok": true})
}

func canRebindFilesWindow(window, remote *process) bool {
	if window == nil || remote == nil || window.kind != "files-window" || window.stdin == nil || window.fileOwner == nil ||
		!remote.fileConnection || remote.mcpOwned || remote.stage != "connected" || remote.credentialKey == "" {
		return false
	}
	select {
	case <-window.done:
		return false
	default:
	}
	owner := window.fileOwner
	if owner.siteID != remote.siteID || owner.credentialKey != remote.credentialKey {
		return false
	}
	if owner == remote {
		return true
	}
	select {
	case <-owner.done:
		return true
	default:
		return false
	}
}

// 呼叫者持有 server.mu；processInput.Write 只入列，不在全域鎖內等待管線。
func queueFilesWindowSession(window, remote *process, address, title string) error {
	input, ok := window.stdin.(*processInput)
	if !ok {
		return errors.New("檔案傳輸視窗的控制通道不可用")
	}
	if err := json.NewEncoder(input).Encode(map[string]string{"event": "files-session", "url": address, "title": title + " · YourDesk Files"}); err != nil {
		return err
	}
	// Admission transfers the window's lifecycle ownership, not permission to
	// resume file I/O. If this window closes before applying the queued message,
	// close its assigned file-only connection too, rather than leave a headless
	// session. Native/JS still require the new instance plus explicit Resume.
	window.fileOwner = remote
	return nil
}
