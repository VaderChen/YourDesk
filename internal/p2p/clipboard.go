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

// 多個檔案讀取工作者共用送出門檻，檢查與送出須序列化。
func (p *Peer) SendClipboard(ctx context.Context, data []byte) error {
	if len(data) > MaxClipboardMessage {
		return errors.New("剪貼簿訊息過大")
	}
	lane := clipboardLane(data)
	if err := p.acquireClipboardSend(ctx, lane); err != nil {
		return err
	}
	defer func() { <-p.clipboardSendGate[lane] }()
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
		p.clipboardWriteMu.Lock()
		screenBuffered := uint64(0)
		if screen != nil {
			screenBuffered = screen.BufferedAmount()
		}
		bufferLimit := uint64(64 * 1024)
		if screenBuffered > 0 {
			bufferLimit = MaxClipboardMessage
		}
		if dc != nil && dc.ReadyState() == webrtc.DataChannelStateOpen && dc.BufferedAmount()+uint64(len(data)) <= bufferLimit &&
			(control == nil || control.BufferedAmount() == 0) && screenBuffered < 256*1024 {
			// 額度僅限制傳送工作者，接收回呼不等磁碟或系統剪貼簿。
			if p.reserveClipboardSlot(lane) {
				err := p.sendData(dc, data)
				if err != nil {
					p.rollbackClipboardSlot(lane)
				}
				p.clipboardWriteMu.Unlock()
				return err
			}
		}
		p.clipboardWriteMu.Unlock()
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

// 等待送出資格也必須能被取消，不能在 mutex 上無限等待。
func (p *Peer) acquireClipboardSend(ctx context.Context, lane int) error {
	p.clipboardSendOnce.Do(func() {
		for i := range p.clipboardSendGate {
			p.clipboardSendGate[i] = make(chan struct{}, 1)
		}
	})
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case p.clipboardSendGate[lane] <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-p.done:
		return errors.New("連線已結束")
	case <-p.clipboardDone:
		return errors.New("剪貼簿通道已結束")
	}
}
