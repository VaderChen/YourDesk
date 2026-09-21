package clientui

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type filesWindowPipe struct{ messages chan []byte }

func (p *filesWindowPipe) Write(data []byte) (int, error) {
	p.messages <- append([]byte(nil), data...)
	return len(data), nil
}
func (p *filesWindowPipe) Close() error { return nil }

func finishedFilesProcess() chan struct{} { done := make(chan struct{}); close(done); return done }

func TestFilesWindowRebindsOnlyAuthenticatedSameTarget(t *testing.T) {
	old := &process{kind: "viewer", siteID: "one", fileConnection: true, credentialKey: "target-key", terminalInstance: "old", done: finishedFilesProcess()}
	remote := &process{kind: "viewer", siteID: "one", fileConnection: true, credentialKey: "target-key", terminalInstance: "new", stage: "connected", done: make(chan struct{})}
	pipe := &filesWindowPipe{messages: make(chan []byte, 2)}
	input := newProcessInput(pipe)
	t.Cleanup(func() { input.Close(); waitProcessInput(t, input.stopped) })
	window := &process{kind: "files-window", fileOwner: old, stdin: input, done: make(chan struct{})}
	s := &server{children: map[string]*process{"viewer:one": remote, "files-window:one:old": window}, token: "local-test-token", origin: "http://127.0.0.1:1234", library: Library{Sites: []Site{{ID: "one", Name: "測試 <站台>"}}}}
	w := httptest.NewRecorder()
	s.openFilesWindow(w, httptest.NewRequest("POST", "/api/files/window", strings.NewReader(`{"session":"viewer:one"}`)))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"reused":true`) {
		t.Fatalf("rebind failed: %d %s", w.Code, w.Body.String())
	}
	if window.fileOwner != remote || len(s.children) != 2 {
		t.Fatal("did not reuse existing window")
	}
	select {
	case data := <-pipe.messages:
		var message struct{ Event, URL string }
		if err := json.Unmarshal(data, &message); err != nil {
			t.Fatal(err)
		}
		u, err := url.Parse(message.URL)
		if err != nil {
			t.Fatal(err)
		}
		fragment, err := url.ParseQuery(u.Fragment)
		if err != nil {
			t.Fatal(err)
		}
		if message.Event != "files-session" || fragment.Get("instance") != "new" || fragment.Get("session") != "viewer:one" || fragment.Get("token") != s.token {
			t.Fatal("wrong session message")
		}
	case <-time.After(time.Second):
		t.Fatal("no rebind message")
	}
	// A second open request never reloads the page or loses the retained queue.
	w = httptest.NewRecorder()
	s.openFilesWindow(w, httptest.NewRequest("POST", "/api/files/window", strings.NewReader(`{"session":"viewer:one"}`)))
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	select {
	case <-pipe.messages:
		t.Fatal("duplicate reconnect resets queue")
	default:
	}
	for _, action := range []string{"resume", "remove", "write"} {
		w = httptest.NewRecorder()
		s.filesAction(w, httptest.NewRequest("POST", "/api/files", strings.NewReader(`{"session":"viewer:one","instance":"old","action":"`+action+`","params":{}}`)))
		if w.Code != 400 {
			t.Fatal("old instance accepted after rebind")
		}
	}
}

func TestFilesWindowDoesNotRebindAcrossIdentityOrLiveOwner(t *testing.T) {
	old := &process{siteID: "one", credentialKey: "target", done: finishedFilesProcess()}
	window := &process{kind: "files-window", fileOwner: old, stdin: &filesWindowPipe{}, done: make(chan struct{})}
	valid := process{siteID: "one", credentialKey: "target", fileConnection: true, stage: "connected", done: make(chan struct{})}
	if !canRebindFilesWindow(window, &valid) {
		t.Fatal("valid rebind rejected")
	}
	for _, change := range []func(*process){func(p *process) { p.siteID = "two" }, func(p *process) { p.credentialKey = "other" }, func(p *process) { p.credentialKey = "" }, func(p *process) { p.mcpOwned = true }, func(p *process) { p.fileConnection = false }, func(p *process) { p.stage = "authenticating" }} {
		other := valid
		change(&other)
		if canRebindFilesWindow(window, &other) {
			t.Fatal("foreign or unauthenticated rebind allowed")
		}
	}
	old.done = make(chan struct{})
	if canRebindFilesWindow(window, &valid) {
		t.Fatal("stole live peer window")
	}
	old.done = finishedFilesProcess()
	window.done = finishedFilesProcess()
	if canRebindFilesWindow(window, &valid) {
		t.Fatal("reused closed window")
	}
}

func TestFilesRebindQueueDoesNotBlockManager(t *testing.T) {
	pipe := newStalledProcessPipe()
	input := newProcessInput(pipe)
	t.Cleanup(func() { input.Close(); waitProcessInput(t, input.stopped) })
	old := &process{}
	remote := &process{}
	window := &process{stdin: input, fileOwner: old}
	started := time.Now()
	if err := queueFilesWindowSession(window, remote, "http://127.0.0.1:1/files.html", "test"); err != nil {
		t.Fatal(err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("blocked manager on pipe")
	}
	waitProcessInput(t, pipe.entered)
	for range processInputCapacity {
		if _, err := input.Write([]byte("{}\n")); err != nil {
			t.Fatal(err)
		}
	}
	other := &process{}
	if err := queueFilesWindowSession(window, other, "http://127.0.0.1:1/files.html", "test"); err == nil {
		t.Fatal("full queue accepted")
	}
	if window.fileOwner != remote {
		t.Fatal("changed owner despite failed enqueue")
	}
}
