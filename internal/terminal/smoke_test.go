//go:build darwin || linux

package terminal

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"yourdesk/internal/p2p"
	"yourdesk/internal/security"
	"yourdesk/internal/signaling"
)

// 真實回環 TLS 配對、WebRTC 控制通道及 PTY；不連接使用者的遠端站台。
func TestLocalP2PTerminalSmoke(t *testing.T) {
	if os.Getenv("YOURDESK_TERMINAL_SMOKE") != "1" {
		t.Skip("需明確啟用本機終端機 Smoke Test")
	}
	t.Setenv("SHELL", "/bin/sh")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	address := l.Addr().String()
	l.Close()
	secretText, e := security.NewSecret()
	if e != nil {
		t.Fatal(e)
	}
	secret, e := security.DecodeSecret(secretText)
	if e != nil {
		t.Fatal(e)
	}
	hosts := make(chan *p2p.Peer, 4)
	errs := make(chan error, 4)
	go func() {
		errs <- signaling.ListenDirect(ctx, address, secret, func(c context.Context, s *signaling.Client) {
			host, e := p2p.NewHost(c, s, func(p2p.Control) {})
			if e != nil {
				errs <- e
				return
			}
			defer host.Close()
			Register(host, func() bool { return c.Err() == nil }, func(bool) {})
			hosts <- host
			select {
			case <-c.Done():
			case <-host.Done():
			}
		})
	}()
	for {
		c, e := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if e == nil {
			c.Close()
			break
		}
		select {
		case e := <-errs:
			t.Fatal(e)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
	connect := func() (*p2p.Peer, *p2p.Peer) {
		t.Helper()
		sig, e := signaling.DialDirect(ctx, address, secret)
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { sig.Close() })
		v, e := p2p.NewViewer(ctx, sig, func(p2p.Frame) {})
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { v.Close() })
		var h *p2p.Peer
		select {
		case h = <-hosts:
		case e := <-errs:
			t.Fatal(e)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		for !v.SupportsCommand("terminal.close") {
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-time.After(20 * time.Millisecond):
			}
		}
		return v, h
	}
	v, h := connect()
	call := func(action string, in any) json.RawMessage {
		t.Helper()
		b, _ := json.Marshal(in)
		out, e := v.CallCommandParams(ctx, "terminal."+action, b)
		if e != nil {
			t.Fatalf("%s: %v", action, e)
		}
		return out.Result
	}
	var ack, seq uint64
	write := func(s string) { t.Helper(); seq++; call("write", request{Data: []byte(s), Sequence: seq}) }
	var output string
	read := func() {
		t.Helper()
		var out struct {
			Sequence uint64
			Data     []byte
			Ended    bool
			Error    string
		}
		if e := json.Unmarshal(call("read", request{Ack: ack}), &out); e != nil {
			t.Fatal(e)
		}
		if out.Sequence > ack {
			output += string(out.Data)
			ack = out.Sequence
		}
		if out.Error != "" {
			t.Fatal(out.Error)
		}
	}
	until := func(needle string) {
		t.Helper()
		deadline := time.Now().Add(12 * time.Second)
		for !strings.Contains(output, needle) {
			read()
			if time.Now().After(deadline) {
				t.Fatalf("等待 %q 逾時，輸出末段 %q", needle, output[max(0, len(output)-400):])
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	call("open", request{Columns: 80, Rows: 24})
	write("stty -echo; printf '\\nREADY_%s\\n' OK\r")
	until("READY_OK")
	output = ""
	write("printf 'PID=%s\\n' $$\r")
	until("PID=")
	for !strings.Contains(output, "\n") {
		read()
	}
	m := regexp.MustCompile(`PID=(\d+)`).FindStringSubmatch(output)
	if len(m) != 2 {
		t.Fatalf("PID: %q", output)
	}
	pid, _ := strconv.Atoi(m[1])
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	work := t.TempDir()
	output = ""
	write("cd '" + work + "'\r")
	write("printf 'CWD=%s\\n' \"$PWD\"; printf 'UNICODE=繁體中文 ✓\\n'\r")
	until("UNICODE=繁體中文 ✓")
	if !strings.Contains(output, "CWD="+work) {
		t.Fatal("工作目錄未保留")
	}
	t.Log("PASS: 配對、P2P、持續工作目錄、UTF-8")
	call("resize", request{Columns: 101, Rows: 37})
	output = ""
	write("stty size\r")
	until("37 101")
	t.Log("PASS: PTY resize 101×37")
	output = ""
	write("sleep 30\r")
	time.Sleep(150 * time.Millisecond)
	write("\x03")
	write("printf 'INTERRUPT_%s\\n' OK\r")
	until("INTERRUPT_OK")
	t.Log("PASS: Ctrl+C 中斷前景程序")
	output = ""
	write("awk 'BEGIN { for(i=0;i<12000;i++) print \"abcdefghijklmnopqrstuvwxyz0123456789\"; print \"BULK_END\" }'\r")
	time.Sleep(300 * time.Millisecond)
	until("BULK_END")
	if n := strings.Count(output, "abcdefghijklmnopqrstuvwxyz0123456789"); n != 12000 {
		t.Fatalf("大量輸出遺失: %d/12000", n)
	}
	t.Log("PASS: 超過 400 KB 輸出與背壓，12000 行完整")
	output = ""
	seq++
	in := request{Data: []byte("printf 'ONCE_%s\\n' OK\r"), Sequence: seq}
	call("write", in)
	call("write", in)
	until("ONCE_OK")
	time.Sleep(100 * time.Millisecond)
	read()
	if strings.Count(output, "ONCE_OK") != 1 {
		t.Fatal("輸入重複執行")
	}
	t.Log("PASS: 重複輸入序號不重複執行")
	call("close", request{})
	deadline := time.Now().Add(3 * time.Second)
	for syscall.Kill(pid, 0) == nil && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if syscall.Kill(pid, 0) == nil {
		t.Fatal("關閉後 Shell 未退出")
	}
	t.Log("PASS: terminal.close 回收 Shell")
	v.Close()
	h.Close()
	time.Sleep(100 * time.Millisecond)
	v, h = connect()
	ack = 0
	seq = 0
	output = ""
	call("open", request{Columns: 80, Rows: 24})
	write("stty -echo; printf '\\nSECOND_%s\\n' OK\r")
	until("SECOND_OK")
	output = ""
	write("printf 'PID=%s\\n' $$\r")
	until("PID=")
	m = regexp.MustCompile(`PID=(\d+)`).FindStringSubmatch(output)
	if len(m) != 2 {
		t.Fatalf("PID: %q", output)
	}
	pid2, _ := strconv.Atoi(m[1])
	t.Cleanup(func() { _ = syscall.Kill(pid2, syscall.SIGKILL) })
	// 模擬連線突然消失，不發送 terminal.close。
	v.Close()
	deadline = time.Now().Add(8 * time.Second)
	for syscall.Kill(pid2, 0) == nil && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if syscall.Kill(pid2, 0) == nil {
		h.Close()
		t.Fatal("斷線後 Shell 未退出")
	}
	t.Log("PASS: 重連及突然斷線回收 Shell")
	fmt.Println("本機對本機 P2P 終端機 Smoke Test 完成")
}
