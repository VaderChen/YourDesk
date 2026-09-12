package p2p

import (
	"context"
	"errors"
	"time"

	"github.com/pion/webrtc/v4"
)

const MaxClipboardMessage = 20 * 1024

func (p *Peer) bindClipboard(dc *webrtc.DataChannel) {
	p.mu.Lock()
	if p.clipboard != nil {
		p.mu.Unlock()
		_ = dc.Close()
		return
	}
	p.clipboard = dc
	p.mu.Unlock()
	dc.OnClose(func() { p.clipboardOnce.Do(func() { close(p.clipboardDone) }) })
	p.onMessage(dc, func(m webrtc.DataChannelMessage) {
		if len(m.Data) > MaxClipboardMessage {
			_ = dc.Close()
			return
		}
		// 回呼只排入有界佇列，檔案 I/O 與圖片轉換由剪貼簿工作者處理。
		select {
		case p.clipboardInbox <- append([]byte(nil), m.Data...):
		default:
			_ = dc.Close()
		}
	})
}

func (p *Peer) ClipboardMessages() <-chan []byte { return p.clipboardInbox }
func (p *Peer) ClipboardDone() <-chan struct{}   { return p.clipboardDone }
func (p *Peer) ClipboardReady() bool {
	p.mu.RLock()
	dc, closed := p.clipboard, p.closed
	p.mu.RUnlock()
	return !closed && dc != nil && dc.ReadyState() == webrtc.DataChannelStateOpen
}

// 依待送緩衝調節，不對檔案固定限速；畫面有積壓時縮小檔案緩衝。
func (p *Peer) SendClipboard(ctx context.Context, data []byte) error {
	if len(data) > MaxClipboardMessage {
		return errors.New("剪貼簿訊息過大")
	}
	ticker := time.NewTicker(2 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		p.mu.RLock()
		dc, control, screen, closed := p.clipboard, p.control, p.screen, p.closed
		p.mu.RUnlock()
		if closed {
			return errors.New("連線已結束")
		}
		screenBuffered := uint64(0)
		if screen != nil {
			screenBuffered = screen.BufferedAmount()
		}
		bufferLimit := uint64(256 * 1024)
		if screenBuffered > 0 {
			bufferLimit = 64 * 1024
		}
		if dc != nil && dc.ReadyState() == webrtc.DataChannelStateOpen && dc.BufferedAmount()+uint64(len(data)) <= bufferLimit &&
			(control == nil || control.BufferedAmount() == 0) && screenBuffered < 256*1024 {
			return p.sendData(dc, data)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-p.done:
			return errors.New("連線已結束")
		case <-p.clipboardDone:
			return errors.New("剪貼簿通道已結束")
		case <-ticker.C:
		}
	}
}
