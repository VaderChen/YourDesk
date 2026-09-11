//go:build windows && !cgo

package main

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"yourdesk/internal/hostsession"
	"yourdesk/internal/security"
	"yourdesk/internal/signaling"
)

var rescueVersion = "YourDesk WinPE Experimental"

type rescueHost struct {
	mu                  sync.Mutex
	room, password, url string
	secret              []byte
	fps, quality        int
	cancel              context.CancelFunc
	done                chan struct{}
	state               string
}

func newRescueHost(url string, fps, quality int) (*rescueHost, error) {
	if _, err := security.SecureSignalURL(url); err != nil {
		return nil, err
	}
	if fps < 1 || fps > 20 || quality < 20 || quality > 90 {
		return nil, errors.New("FPS must be 1-20; quality must be 20-90")
	}
	var random [13]byte
	if _, err := rand.Read(random[:]); err != nil {
		return nil, err
	}
	id := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(random[:])[:20]
	groups := []string{"YD"}
	for i := 0; i < len(id); i += 4 {
		groups = append(groups, id[i:i+4])
	}
	password, err := security.NewSecret()
	if err != nil {
		return nil, err
	}
	secret, err := security.DecodeSecret(password)
	if err != nil {
		return nil, err
	}
	return &rescueHost{room: strings.Join(groups, "-"), password: password, secret: secret, url: url, fps: fps, quality: quality, state: "Stopped / 已停止"}, nil
}
func (h *rescueHost) status() (string, bool, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.state, h.cancel != nil, h.done != nil
}
func (h *rescueHost) setStatus(ctx context.Context, value string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cancel != nil && ctx.Err() == nil {
		h.state = value
	}
}
func (h *rescueHost) start() {
	h.mu.Lock()
	// 等前一輪完整回收再允許啟動，防止兩個 Host 同時接收。
	if h.done != nil {
		h.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	h.done = make(chan struct{})
	done := h.done
	h.state = "Connecting / 正在連線"
	h.mu.Unlock()
	go func() {
		defer func() {
			h.mu.Lock()
			h.cancel = nil
			h.done = nil
			h.state = "Stopped / 已停止"
			close(done)
			h.mu.Unlock()
		}()
		for ctx.Err() == nil {
			dial, finish := context.WithTimeout(ctx, 20*time.Second)
			sig, err := signaling.Dial(dial, h.url, h.room, signaling.RoleHost, h.secret)
			finish()
			if err == nil {
				h.setStatus(ctx, "Waiting for connection / 等待遠端連線")
				sessionCtx, finishSession := context.WithCancel(ctx)
				err = hostsession.Stream(sessionCtx, sig, hostsession.Options{Version: rescueVersion, Codec: "software-jpeg", FPS: h.fps, Quality: h.quality, PrimaryDisplayOnly: true, DisableClipboard: true, DisableRemoteData: true, OnState: func(state string) {
					if state == "capture-unavailable" {
						h.setStatus(sessionCtx, "Capture unavailable / 畫面擷取不可用")
					} else if state == "capture-restored" {
						h.setStatus(sessionCtx, "Capture ready / 畫面擷取恢復")
					} else if state == "connected" {
						h.setStatus(sessionCtx, "Connected / 遠端已連線")
					} else {
						h.setStatus(sessionCtx, "Disconnected / 遠端已斷線")
					}
				}})
				finishSession()
				sig.Close()
			}
			if ctx.Err() != nil {
				return
			}
			if err != nil && !errors.Is(err, io.EOF) {
				// 僅顯示概要；不記錄密碼、配對 envelope 或網路密鑰。
				h.setStatus(ctx, "Connection failed; retrying / 連線失敗，將重試")
				slog.Warn("WinPE connection retry")
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
			}
		}
	}()
}
func (h *rescueHost) stop() {
	h.mu.Lock()
	cancel := h.cancel
	h.cancel = nil
	if h.done != nil {
		h.state = "Stopping / 正在停止"
	}
	h.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	hostsession.Disconnect()
}
