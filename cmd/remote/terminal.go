package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"yourdesk/internal/agentremote"
	"yourdesk/internal/p2p"
	"yourdesk/internal/peertransport"
	"yourdesk/internal/signaling"
)

// 命令列入口不建立桌面視窗、解碼器或剪貼簿同步。
func runTerminal(address, room string, mode peertransport.Mode) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	passwords := newPasswordInput()
	go func() {
		select {
		case <-passwords.done:
			cancel()
		case <-ctx.Done():
		}
	}()
	secret, e := passwords.read(ctx, false)
	if e != nil {
		return e
	}
	var sig *signaling.Client
	if direct, ok := signaling.DirectAddress(room); ok {
		sig, e = signaling.DialDirect(ctx, direct, secret)
	} else {
		sig, e = signaling.Dial(ctx, address, room, signaling.RoleViewer, secret)
	}
	if e != nil {
		return e
	}
	defer sig.Close()
	sig.ResolveSecret = passwords.resolver()
	setup, end := context.WithTimeout(ctx, 90*time.Second)
	defer end()
	peer, e := p2p.NewViewerWithTransport(setup, sig, mode, func(p2p.Frame) {}, func(p2p.Control) {})
	if e != nil {
		return e
	}
	defer peer.Close()
	emitUIEvent("authenticated", "")
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	var connectedAt time.Time
	for !peer.SupportsCommand("terminal.open") {
		if peer.Connected() {
			if connectedAt.IsZero() {
				connectedAt = time.Now()
			}
			if time.Since(connectedAt) > 8*time.Second {
				return fmt.Errorf("對方尚未支援命令列，請更新對方的 YourDesk 後再試。")
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-peer.Done():
			return fmt.Errorf("遠端連線已結束")
		case <-setup.Done():
			return fmt.Errorf("對方尚未支援命令列，請更新對方的 YourDesk 後再試。")
		case <-tick.C:
		}
	}
	emitUIEvent("frame", "")
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-peer.Done():
			return nil
		case req := <-passwords.agent:
			out := agentremote.Response{ID: req.ID}
			if !strings.HasPrefix(req.Action, "terminal.") {
				out.Error = "命令列連線不接受桌面操作"
			} else {
				call, stop := context.WithDeadline(ctx, time.UnixMilli(req.Expires))
				res, err := peer.CallCommandParams(call, req.Action, req.Params)
				stop()
				if err != nil {
					out.Error = err.Error()
				} else {
					out.Result = res.Result
				}
			}
			raw, _ := json.Marshal(out)
			fmt.Fprintln(os.Stdout, agentremote.Prefix+string(raw))
			if req.Action == "terminal.close" {
				return nil
			}
		}
	}
}
