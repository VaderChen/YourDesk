//go:build darwin || linux

package clientui

import (
	"context"
	"encoding/json"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
	"yourdesk/internal/p2p"
	"yourdesk/internal/security"
	"yourdesk/internal/signaling"
	"yourdesk/internal/terminal"
)

// 瀏覽器 → 正式 UI API → 真實 Remote helper → TLS/P2P → PTY，不使用 API 模擬。
func TestTerminalBrowserEndToEnd(t *testing.T) {
	binary := os.Getenv("YOURDESK_TERMINAL_SMOKE_BINARY")
	if binary == "" || os.Getenv("YOURDESK_PLAYWRIGHT_MODULE") == "" {
		t.Skip("需指定 Remote binary 與 Playwright")
	}
	t.Setenv("SHELL", "/bin/sh")
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	app := &server{children: map[string]*process{}, token: "local-e2e-token", viewerClosed: make(chan struct{}, 2), connected: make(chan struct{}, 2)}
	t.Cleanup(func() {
		app.mu.Lock()
		for _, p := range app.children {
			_ = p.cmd.Process.Kill()
		}
		app.mu.Unlock()
	})
	for _, id := range []string{"one", "two"} {
		l, e := net.Listen("tcp", "127.0.0.1:0")
		if e != nil {
			t.Fatal(e)
		}
		address := l.Addr().String()
		l.Close()
		secretText, _ := security.NewSecret()
		secret, _ := security.DecodeSecret(secretText)
		go signaling.ListenDirect(ctx, address, secret, func(c context.Context, s *signaling.Client) {
			h, e := p2p.NewHost(c, s, func(p2p.Control) {})
			if e != nil {
				return
			}
			defer h.Close()
			terminal.Register(h, func() bool { return c.Err() == nil }, func(bool) {})
			select {
			case <-c.Done():
			case <-h.Done():
			}
		})
		for {
			conn, e := net.DialTimeout("tcp", address, 50*time.Millisecond)
			if e == nil {
				conn.Close()
				break
			}
			if ctx.Err() != nil {
				t.Fatal(ctx.Err())
			}
			time.Sleep(20 * time.Millisecond)
		}
		app.mu.Lock()
		e = app.start("viewer", id, binary, []string{"-room", address, "-secret-stdin", "-terminal"})
		if e != nil {
			app.mu.Unlock()
			t.Fatal(e)
		}
		p := app.children["viewer:"+id]
		p.terminalConnection = true
		p.terminalInstance = id
		e = json.NewEncoder(p.stdin).Encode(map[string]string{"secret": secretText})
		app.mu.Unlock()
		if e != nil {
			t.Fatal(e)
		}
	}
	for {
		app.mu.Lock()
		ready := len(app.children) == 2
		for _, p := range app.children {
			ready = ready && p.stage == "connected"
		}
		app.mu.Unlock()
		if ready {
			break
		}
		if ctx.Err() != nil {
			t.Fatal(ctx.Err())
		}
		time.Sleep(20 * time.Millisecond)
	}
	web, _ := fs.Sub(assets, "web")
	mux := http.NewServeMux()
	mux.HandleFunc("/api/", app.api)
	mux.Handle("/", http.FileServer(http.FS(web)))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'")
		mux.ServeHTTP(w, r)
	}))
	defer srv.Close()
	app.origin = srv.URL
	script, _ := filepath.Abs("../../scripts/smoke-terminal-e2e.cjs")
	cmd := exec.CommandContext(ctx, "node", script)
	cmd.Env = append(os.Environ(), "YOURDESK_TERMINAL_TEST_URL="+srv.URL)
	out, e := cmd.CombinedOutput()
	t.Log(string(out))
	if e != nil {
		t.Fatal(e)
	}
}
