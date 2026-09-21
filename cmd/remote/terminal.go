package main

import (
	"context"
	"encoding/json"
	"errors"
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
	return runCommandSession(address, room, mode, false)
}

// 傳檔與終端機共用認證與生命週期，但不開 Shell、桌面或剪貼簿。
func runCommandSession(address, room string, mode peertransport.Mode, files bool) error {
	method, prefix := "terminal.open", "terminal."
	unsupported := "對方尚未支援命令列，請更新對方的 YourDesk 後再試。"
	if files {
		method, prefix = "xfer.list", "xfer."
		unsupported = "對方尚未支援檔案傳輸，請更新對方的 YourDesk 後再試。"
	}
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
	var peer *p2p.Peer
	if files {
		peer, e = p2p.NewFilesViewerWithTransport(setup, sig, mode)
	} else {
		peer, e = p2p.NewViewerWithTransport(setup, sig, mode, func(p2p.Frame) {}, func(p2p.Control) {})
	}
	if e != nil {
		return e
	}
	defer peer.Close()
	emitUIEvent("authenticated", "")
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	var connectedAt time.Time
	for !peer.SupportsCommand(method) {
		if peer.Connected() {
			if connectedAt.IsZero() {
				connectedAt = time.Now()
			}
			if time.Since(connectedAt) > 8*time.Second {
				return fmt.Errorf("%s", unsupported)
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-peer.Done():
			return fmt.Errorf("遠端連線已結束")
		case <-setup.Done():
			return fmt.Errorf("%s", unsupported)
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
			if !strings.HasPrefix(req.Action, prefix) {
				out.Error = "此連線不接受其他模式的操作"
			} else {
				call, stop := context.WithDeadline(ctx, time.UnixMilli(req.Expires))
				res, err := peer.CallCommandParams(call, req.Action, req.Params)
				stop()
				out = commandAgentResponse(req.ID, res, err)
			}
			raw, _ := json.Marshal(out)
			fmt.Fprintln(os.Stdout, agentremote.Prefix+string(raw))
			if req.Action == "terminal.close" {
				return nil
			}
		}
	}
}

func commandAgentResponse(id string, res p2p.CommandResponse, err error) agentremote.Response {
	out := agentremote.Response{ID: id}
	if err == nil {
		out.Result = res.Result
		return out
	}
	out.Error, out.Code = err.Error(), res.Code
	if errors.Is(err, p2p.ErrCommandInterrupted) {
		out.Code = agentremote.CodeRequestInterrupted
	}
	return out
}
