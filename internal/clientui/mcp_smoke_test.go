//go:build darwin || linux

package clientui

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"yourdesk/internal/p2p"
	"yourdesk/internal/security"
	"yourdesk/internal/signaling"
	"yourdesk/internal/terminal"
)

// MCP HTTP → 正式 connect API → Remote 子程序 → TLS/P2P → 真實 PTY。
func TestMCPTerminalSmoke(t *testing.T) {
	binary := os.Getenv("YOURDESK_TERMINAL_SMOKE_BINARY")
	if binary == "" {
		t.Skip("需指定 Remote 執行檔")
	}
	t.Setenv("SHELL", "/bin/sh")
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	dir := t.TempDir()
	if err := os.Symlink(binary, filepath.Join(dir, "yourdesk-remote")); err != nil {
		t.Fatal(err)
	}
	app := &server{children: map[string]*process{}, token: "smoke-local", executable: filepath.Join(dir, "client"), configPath: filepath.Join(dir, "sites.json"), viewerClosed: make(chan struct{}, 4), connected: make(chan struct{}, 4)}
	defer func() {
		app.mu.Lock()
		defer app.mu.Unlock()
		for _, p := range app.children {
			_ = p.cmd.Process.Kill()
		}
	}()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := l.Addr().String()
	l.Close()
	secretText, _ := security.NewSecret()
	secret, _ := security.DecodeSecret(secretText)
	go signaling.ListenDirect(ctx, address, secret, func(c context.Context, s *signaling.Client) {
		peer, e := p2p.NewHost(c, s, func(p2p.Control) {})
		if e != nil {
			return
		}
		defer peer.Close()
		terminal.Register(peer, func() bool { return c.Err() == nil }, func(bool) {})
		select {
		case <-c.Done():
		case <-peer.Done():
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
	server := app.mcpServer()
	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true}))
	defer httpServer.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "smoke", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: httpServer.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, tool := range tools.Tools {
		found[tool.Name] = true
	}
	for _, name := range []string{"connect", "get_status", "get_site_capabilities", "remote_terminal", "disconnect"} {
		if !found[name] {
			t.Fatal("missing", name)
		}
	}
	call := func(name string, args map[string]any, wantError bool) map[string]any {
		t.Helper()
		r, e := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if e != nil {
			t.Fatal(e)
		}
		if r.IsError != wantError {
			t.Fatalf("%s error=%v content=%v", name, r.IsError, r.Content)
		}
		if wantError {
			return nil
		}
		var out map[string]any
		if r.StructuredContent != nil {
			b, _ := json.Marshal(r.StructuredContent)
			_ = json.Unmarshal(b, &out)
		}
		if out == nil {
			for _, c := range r.Content {
				if text, ok := c.(*mcp.TextContent); ok {
					_ = json.Unmarshal([]byte(text.Text), &out)
				}
			}
		}
		return out
	}
	call("get_site_capabilities", map[string]any{}, false)
	call("connect", map[string]any{"room": address, "secret": secretText, "terminal": true}, false)
	instance := ""
	for {
		status := call("get_status", map[string]any{}, false)
		for _, v := range status["processes"].([]any) {
			p := v.(map[string]any)
			if p["session"] == "quick" && p["stage"] == "connected" {
				instance = p["instance"].(string)
			}
		}
		if instance != "" {
			break
		}
		if ctx.Err() != nil {
			t.Fatal(ctx.Err())
		}
		time.Sleep(30 * time.Millisecond)
	}
	action := func(kind string, params map[string]any) map[string]any {
		params["session"] = "quick"
		params["instance"] = instance
		params["action"] = kind
		return call("remote_terminal", params, false)
	}
	call("remote_terminal", map[string]any{"session": "quick", "instance": "stale", "action": "open"}, true)
	action("open", map[string]any{})
	action("resize", map[string]any{"columns": 101, "rows": 37})
	action("write", map[string]any{"text": "printf 'MCP_%s\\n' OK; stty size\r", "sequence": 1})
	var ack float64
	var output strings.Builder
	for {
		r := action("read", map[string]any{"ack": ack})
		if data, ok := r["data"].(string); ok {
			b, e := base64.StdEncoding.DecodeString(data)
			if e != nil {
				t.Fatal(e)
			}
			output.Write(b)
		}
		if n, ok := r["sequence"].(float64); ok {
			ack = n
		}
		if strings.Contains(output.String(), "MCP_OK") && strings.Contains(output.String(), "37 101") {
			break
		}
		if ctx.Err() != nil {
			t.Fatal("missing terminal output")
		}
		time.Sleep(30 * time.Millisecond)
	}
	action("close", map[string]any{})
	for {
		app.mu.Lock()
		left := app.children["quick"] != nil
		app.mu.Unlock()
		if !left {
			break
		}
		if ctx.Err() != nil {
			t.Fatal("connection not cleaned up")
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Log("MCP HTTP discovery/connect/status, stale instance rejection, PTY open/write/read/resize/close and cleanup passed")
}
