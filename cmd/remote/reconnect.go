package main

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"yourdesk/internal/peertransport"
	retry "yourdesk/internal/reconnect"
	"yourdesk/internal/signaling"
)

type reconnectResult struct {
	connection *viewerConnection
	err        error
}
type viewerReconnect struct {
	ctx     context.Context
	cancel  context.CancelFunc
	begin   func(*viewerConnection)
	results chan reconnectResult
	running bool
}

func newViewerReconnect(parent context.Context, g *game, url, room string, secret []byte, codec string, mode peertransport.Mode) *viewerReconnect {
	ctx, cancel := context.WithCancel(parent)
	r := &viewerReconnect{ctx: ctx, cancel: cancel, results: make(chan reconnectResult)}
	// 密碼只留在此程序記憶體，重試不呼叫互動式 resolver。
	secret = append([]byte(nil), secret...)
	r.begin = func(previous *viewerConnection) {
		r.running = true
		previous.Close()
		go func() {
			old := previous
			var connected *viewerConnection
			err := retry.Run(ctx, func(attempt context.Context) error {
				select {
				case <-attempt.Done():
					return attempt.Err()
				case <-old.done:
				}
				var sig *signaling.Client
				var err error
				if address, direct := signaling.DirectAddress(room); direct {
					sig, err = signaling.DialDirect(attempt, address, secret)
				} else {
					sig, err = signaling.Dial(attempt, url, room, signaling.RoleViewer, secret)
				}
				if err != nil {
					return err
				}
				next, err := openViewerConnection(ctx, attempt, sig, g, codec, mode, true)
				if next != nil {
					old = next
				}
				if err != nil {
					return err
				}
				select {
				case <-attempt.Done():
					err = attempt.Err()
				case <-next.peer.Done():
					err = errors.New("重連在收到畫面前結束")
				case <-next.ready:
					if attempt.Err() == nil && next.peer.Connected() {
						connected = next
						return nil
					}
					err = errors.New("重連尚未完成")
				}
				next.Close()
				return err
			})
			select {
			case <-ctx.Done():
				if connected != nil {
					connected.Close()
				}
			case r.results <- reconnectResult{connected, err}:
			}
		}()
	}
	return r
}

// 只由 UI 執行緒切換目前連線；舊連線資源已清理完成。
func (g *game) updateReconnect() error {
	r := g.reconnect
	if r == nil {
		return nil
	}
	if !viewerAutoReconnect.Load() {
		if r.running {
			r.cancel()
			return ebiten.Termination
		}
		return nil
	}
	if !r.running {
		g.mu.Lock()
		g.displayKnown, g.displayPending = false, false
		g.displayRequest = 0
		g.agentViewID, g.displayedViewID = 0, 0
		g.agentPaused, g.agentViewChanging = false, false
		g.mu.Unlock()
		g.streamCapabilities.Store(nil)
		g.streamResult.Store(nil)
		g.rawSupported.Store(false)
		g.enhancementSupported.Store(false)
		r.begin(g.connection)
		emitUIEvent("reconnecting", "")
	}
	select {
	case result := <-r.results:
		r.running = false
		if result.err != nil {
			slog.Warn("背景重連三次失敗，關閉遠端視窗")
			return ebiten.Termination
		}
		g.mu.Lock()
		g.connection = result.connection
		g.peer = result.connection.peer
		g.clipboard = result.connection.clipboard
		g.mu.Unlock()
		g.disconnected = false
		clear(g.lastKeys)
		clear(g.lastButtons)
		clear(g.rawHeld)
		g.restoreDisplay = true
		g.streamRevision = 0
		g.streamRequest.Revision = 0
		g.qualitySent = -1
		g.qualitySentAt = time.Time{}
		g.ml.reset()
		g.interpolation.reset()
		ebiten.SetWindowTitle(g.windowTitle)
		emitUIEvent("reconnected", "")
	default:
	}
	return nil
}
