//go:build darwin || windows

package clientui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
	"time"
)

// 真實原生 WebView 生命週期，需明確提供測試 Client 執行檔。
func TestNativeTerminalWindows(t *testing.T) {
	binary := os.Getenv("YOURDESK_TERMINAL_WINDOW_BINARY")
	if binary == "" {
		t.Skip("需指定原生視窗測試執行檔")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	ready := make(chan string, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/terminal.html":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<!doctype html><title>YourDesk 視窗測試</title><p>獨立終端機視窗生命週期測試，會自動關閉。</p><script src="/test.js"></script>`)
		case "/test.js":
			w.Header().Set("Content-Type", "application/javascript")
			fmt.Fprint(w, `const id=location.hash.slice(1);fetch('/ready?id='+id);if(id==='one')setTimeout(()=>window.yourdeskCloseTerminal(),1500);`)
		case "/ready":
			ready <- r.URL.Query().Get("id")
			w.WriteHeader(204)
		}
	}))
	defer srv.Close()
	type child struct {
		cmd   *exec.Cmd
		stdin io.WriteCloser
		exit  chan error
	}
	var children []child
	for _, id := range []string{"one", "two"} {
		cmd := exec.CommandContext(ctx, binary, "--terminal-window")
		in, e := cmd.StdinPipe()
		if e != nil {
			t.Fatal(e)
		}
		cmd.Stderr = os.Stderr
		if e = cmd.Start(); e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { in.Close(); _ = cmd.Process.Kill() })
		if e = json.NewEncoder(in).Encode(map[string]string{"url": srv.URL + "/terminal.html#" + id, "title": "YourDesk · 視窗測試 " + id}); e != nil {
			t.Fatal(e)
		}
		exited := make(chan error, 1)
		go func() { exited <- cmd.Wait() }()
		children = append(children, child{cmd, in, exited})
	}
	for range 2 {
		select {
		case <-ready:
		case <-ctx.Done():
			t.Fatal("原生視窗未載入")
		}
	}
	select {
	case e := <-children[0].exit:
		if e != nil {
			t.Fatal(e)
		}
	case <-ctx.Done():
		t.Fatal("第一個視窗未關閉")
	}
	select {
	case e := <-children[1].exit:
		t.Fatalf("第二個視窗受到影響: %v", e)
	default:
	}
	children[1].stdin.Close()
	select {
	case e := <-children[1].exit:
		if e != nil {
			t.Fatal(e)
		}
	case <-ctx.Done():
		t.Fatal("父管線關閉後視窗未退出")
	}
	t.Log("PASS: 兩個原生視窗獨立載入；關閉一個不影響另一個，父管線 EOF 可回收")
}
