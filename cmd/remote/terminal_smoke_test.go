//go:build darwin || linux

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
	"yourdesk/internal/hostsession"
	"yourdesk/internal/p2p"
	"yourdesk/internal/security"
	"yourdesk/internal/signaling"
)

func TestTerminalHelperSmoke(t *testing.T) {
	binary := os.Getenv("YOURDESK_TERMINAL_SMOKE_BINARY")
	if binary == "" {
		t.Skip("需指定 Smoke Test helper")
	}
	for _, supported := range []bool{true, false} {
		name := "legacy"
		if supported {
			name = "supported"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()
			l, e := net.Listen("tcp", "127.0.0.1:0")
			if e != nil {
				t.Fatal(e)
			}
			addr := l.Addr().String()
			l.Close()
			text, e := security.NewSecret()
			if e != nil {
				t.Fatal(e)
			}
			secret, _ := security.DecodeSecret(text)
			errs := make(chan error, 2)
			go func() {
				errs <- signaling.ListenDirect(ctx, addr, secret, func(c context.Context, s *signaling.Client) {
					if supported {
						errs <- hostsession.Stream(c, s, hostsession.Options{Headless: true})
						return
					}
					h, e := p2p.NewHost(c, s, func(p2p.Control) {})
					if e != nil {
						errs <- e
						return
					}
					defer h.Close()
					select {
					case <-c.Done():
					case <-h.Done():
					}
				})
			}()
			for {
				c, e := net.DialTimeout("tcp", addr, 50*time.Millisecond)
				if e == nil {
					c.Close()
					break
				}
				select {
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				case e := <-errs:
					t.Fatal(e)
				case <-time.After(20 * time.Millisecond):
				}
			}
			cmd := exec.CommandContext(ctx, binary, "-room", addr, "-secret-stdin", "-terminal")
			stdin, e := cmd.StdinPipe()
			if e != nil {
				t.Fatal(e)
			}
			stdout, e := cmd.StdoutPipe()
			if e != nil {
				t.Fatal(e)
			}
			cmd.Stderr = os.Stderr
			if e = cmd.Start(); e != nil {
				t.Fatal(e)
			}
			defer cmd.Process.Kill()
			defer stdin.Close()
			json.NewEncoder(stdin).Encode(map[string]string{"secret": text})
			lines := make(chan string, 64)
			go func() {
				defer close(lines)
				s := bufio.NewScanner(stdout)
				for s.Scan() {
					lines <- s.Text()
				}
			}()
			frame, notice := false, false
			for line := range lines {
				if strings.Contains(line, `"event":"frame"`) {
					frame = true
					if supported {
						stdin.Close()
					}
				}
				if strings.Contains(line, "對方尚未支援命令列，請更新對方的 YourDesk 後再試。") {
					notice = true
				}
			}
			e = cmd.Wait()
			if ctx.Err() != nil {
				t.Fatal("Helper 未及時結束")
			}
			if supported {
				if !frame {
					t.Fatal("未送出終端機就緒事件")
				}
				t.Log("PASS: 真實 Remote helper 協商成功，關閉 stdin 後退出")
			} else {
				if e == nil || !notice || frame {
					t.Fatalf("舊版處理不符: error=%v notice=%v frame=%v", e, notice, frame)
				}
				t.Log("PASS: 舊版缺少 terminal 能力，顯示更新提示、未進入桌面／終端機")
			}
		})
	}
}
