package clientui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRemoteActionRejectsPrivateNamespaces(t *testing.T) {
	pipe := newStalledProcessPipe()
	defer pipe.Close()
	p := &process{kind: "viewer", stage: "connected", stdin: pipe, terminalConnection: true, terminalInstance: "user-window", done: make(chan struct{})}
	app := &server{children: map[string]*process{"viewer:user": p}}
	srv := app.mcpServer()
	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true}))
	defer httpServer.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "boundary-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: httpServer.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	for _, action := range []string{"terminal.read", "terminal.close", "terminal.write", "shell.run", "files.read", "video.snapshot", "network.test", "unknown", ""} {
		out, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "remote_action", Arguments: map[string]any{"session": "viewer:user", "action": action}})
		if err != nil || !out.IsError {
			t.Fatalf("%q 未被拒絕：%v %+v", action, err, out)
		}
	}
	select {
	case <-pipe.entered:
		t.Fatal("未授權 namespace 被送入子程序")
	default:
	}
}
