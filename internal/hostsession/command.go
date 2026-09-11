package hostsession

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"
	"yourdesk/internal/p2p"
	"yourdesk/internal/remotedata"
	"yourdesk/internal/signaling"
	"yourdesk/internal/terminal"
)

// 同一個 Host 的無桌面分支，在建立任何圖像資源之前選用。
func commandSession(ctx context.Context, sig *signaling.Client, options Options) error {
	peer, err := p2p.NewHostWithTransport(ctx, sig, options.Transport, func(p2p.Control) {})
	if err != nil {
		return err
	}
	defer peer.Close()
	if !activeSession.CompareAndSwap(false, true) {
		return fmt.Errorf("Client 已有遠端工作階段")
	}
	defer activeSession.Store(false)
	incomingPeer.Store(peer)
	defer incomingPeer.CompareAndSwap(peer, nil)
	var authorized atomic.Bool
	authorized.Store(true)
	defer authorized.Store(false)
	if !options.DisableRemoteData {
		terminal.Register(peer, authorized.Load, func(bool) {})
		remotedata.Register(peer, authorized.Load)
	}
	_ = peer.RegisterCommand("session.capabilities", func(context.Context) (any, error) {
		return map[string]any{"schema": 1, "desktop": false, "terminal": terminal.Available(), "clipboard": false}, nil
	})
	generation := incomingGeneration.Add(1)
	connected := false
	defer func() {
		if connected {
			if options.OnState != nil {
				options.OnState("disconnected")
			}
			fmt.Printf("YOURDESK_UI_EVENT {\"event\":\"host-disconnected\",\"session\":%d}\n", generation)
		}
	}()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-peer.Done():
			return nil
		case <-ticker.C:
			if peer.Connected() {
				if !connected {
					connected = true
					if options.OnState != nil {
						options.OnState("connected")
					}
					fmt.Printf("YOURDESK_UI_EVENT {\"event\":\"host-connected\",\"session\":%d}\n", generation)
				}
				// 桌面 Viewer 即使跳過 Server 查詢，仍會從已驗證的 P2P 通道收到能力限制。
				_ = peer.SendControl(p2p.Control{Type: "desktop-unavailable"})
			}
		}
	}
}
