package clientui

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"yourdesk/internal/agentremote"
)

// Exercise the authenticated HTTP → private process bridge without a real
// remote peer or filesystem operation. In particular, partial-delete results
// must remain structured JSON, not get flattened into an ordinary HTTP error.
func TestFilesMutationResultsSurvivePrivateBridge(t *testing.T) {
	for _, tc := range []struct{ action, params, result string }{
		{"mkdir", `{"path":"fixture/new"}`, `{"ok":true}`},
		{"resume", `{"id":"test-id","token":"test-token","path":"fixture/data","size":10}`, `{"state":"uploading","nextOffset":5}`},
		{"remove", `{"path":"fixture/data","directory":false,"modified":"version","size":10,"confirm":"data","recursive":false}`, `{"ok":true,"removedCount":1}`},
		{"remove", `{"path":"fixture/tree","directory":true,"modified":"version","size":0,"confirm":"tree","recursive":true}`, `{"ok":false,"partial":true,"removedCount":2,"error":"interrupted"}`},
	} {
		t.Run(tc.action, func(t *testing.T) {
			pipe := &filesWindowPipe{messages: make(chan []byte, 1)}
			input := newProcessInput(pipe)
			t.Cleanup(func() { input.Close(); waitProcessInput(t, input.stopped) })
			remote := &process{kind: "viewer", fileConnection: true, stage: "connected", terminalInstance: "current", stdin: input, done: make(chan struct{})}
			s := &server{children: map[string]*process{"viewer:one": remote}, token: "http-test-token", origin: "http://127.0.0.1:1234"}
			payload, _ := json.Marshal(map[string]any{"session": "viewer:one", "instance": "current", "action": tc.action, "params": json.RawMessage(tc.params)})
			r := httptest.NewRequest("POST", "/api/files", bytes.NewReader(payload))
			r.Header.Set("X-YourDesk-Token", s.token)
			r.Header.Set("Origin", s.origin)
			w := httptest.NewRecorder()
			done := make(chan struct{})
			go func() { defer close(done); s.api(w, r) }()
			var request struct{ Agent agentremote.Request }
			select {
			case data := <-pipe.messages:
				if err := json.Unmarshal(data, &request); err != nil {
					t.Fatal(err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("authenticated action was not forwarded")
			}
			if request.Agent.Action != "xfer."+tc.action || string(request.Agent.Params) != tc.params || request.Agent.Expires <= time.Now().UnixMilli() {
				t.Fatal("private action contract changed")
			}
			s.mu.Lock()
			pending := remote.agentPending[request.Agent.ID]
			s.mu.Unlock()
			if pending == nil {
				t.Fatal("missing private response receiver")
			}
			pending <- agentremote.Response{ID: request.Agent.ID, Result: json.RawMessage(tc.result)}
			waitProcessInput(t, done)
			if w.Code != 200 || !bytes.Equal(bytes.TrimSpace(w.Body.Bytes()), []byte(tc.result)) {
				t.Fatalf("result lost at bridge: %d %s", w.Code, w.Body.String())
			}
			s.mu.Lock()
			owned, outstanding := remote.mcpOwned, len(remote.agentPending)
			s.mu.Unlock()
			if owned || outstanding != 0 {
				t.Fatal("file operation changed ownership or leaked pending response")
			}
		})
	}
}

func TestFilesUnavailableCodeDistinguishesConnectionFromFileErrors(t *testing.T) {
	for _, stage := range []string{"", "authenticating", "disconnected", "reconnecting"} {
		remote := &process{kind: "viewer", fileConnection: true, stage: stage, terminalInstance: "current", done: make(chan struct{})}
		s := &server{children: map[string]*process{"viewer:one": remote}}
		w := httptest.NewRecorder()
		s.filesAction(w, httptest.NewRequest("POST", "/api/files", bytes.NewBufferString(`{"session":"viewer:one","instance":"current","action":"stat","params":{"path":"sample"}}`)))
		var failure struct{ Code string }
		if w.Code != 400 || json.Unmarshal(w.Body.Bytes(), &failure) != nil || failure.Code != "files_session_unavailable" {
			t.Fatalf("missing reconnect hint: %d %s", w.Code, w.Body.String())
		}
	}
}

func TestFilesFailureClassificationSurvivesPrivateBridge(t *testing.T) {
	for _, tc := range []struct{ name, responseCode, wantCode string }{
		{"remote-interrupted", agentremote.CodeRequestInterrupted, "files_request_interrupted"},
		{"remote-missing", "failed", ""},
		{"remote-denied", "failed", ""},
		{"remote-invalid", "invalid_request", ""},
		{"remote-unsupported", "unsupported_method", ""},
		{"untyped-deadline-message", "", ""},
		{"local-timeout", "", "files_request_interrupted"},
		{"local-pipe-closed", "", "files_request_interrupted"},
		{"local-pending-full", "", "files_request_interrupted"},
		{"remote-ended", "", "files_session_unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pipe := &filesWindowPipe{messages: make(chan []byte, 1)}
			input := newProcessInput(pipe)
			t.Cleanup(func() { input.Close(); waitProcessInput(t, input.stopped) })
			remote := &process{kind: "viewer", fileConnection: true, stage: "connected", terminalInstance: "current", stdin: input, done: make(chan struct{})}
			s := &server{children: map[string]*process{"viewer:one": remote}, token: "http-test-token", origin: "http://127.0.0.1:1234"}
			if tc.name == "local-pending-full" {
				remote.agentPending = make(map[string]chan agentremote.Response)
				for _, id := range []string{"1", "2", "3", "4", "5", "6", "7", "8"} {
					remote.agentPending[id] = make(chan agentremote.Response, 1)
				}
			}
			if tc.name == "local-pipe-closed" {
				input.Close()
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			r := httptest.NewRequest("POST", "/api/files", bytes.NewBufferString(`{"session":"viewer:one","instance":"current","action":"stat","params":{"path":"sample"}}`)).WithContext(ctx)
			r.Header.Set("X-YourDesk-Token", s.token)
			r.Header.Set("Origin", s.origin)
			w := httptest.NewRecorder()
			done := make(chan struct{})
			go func() { defer close(done); s.api(w, r) }()
			if tc.name != "local-pending-full" && tc.name != "local-pipe-closed" {
				var request struct{ Agent agentremote.Request }
				select {
				case data := <-pipe.messages:
					if err := json.Unmarshal(data, &request); err != nil {
						t.Fatal(err)
					}
				case <-time.After(2 * time.Second):
					t.Fatal("file stat did not reach private bridge")
				}
				s.mu.Lock()
				pending := remote.agentPending[request.Agent.ID]
				s.mu.Unlock()
				switch tc.name {
				case "local-timeout":
					cancel()
				case "remote-ended":
					close(remote.done)
				default:
					// The wording deliberately contains timeout language even for
					// permanent failures: classification must only use the code.
					pending <- agentremote.Response{ID: request.Agent.ID, Code: tc.responseCode, Error: "deadline exceeded / source missing or denied"}
				}
			}
			waitProcessInput(t, done)
			var failure struct{ Code, Error string }
			if w.Code != 400 || json.Unmarshal(w.Body.Bytes(), &failure) != nil || failure.Code != tc.wantCode || failure.Error == "" {
				t.Fatalf("wrong file error class: %d %s", w.Code, w.Body.String())
			}
		})
	}
}
