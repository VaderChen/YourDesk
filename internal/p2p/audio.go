package p2p

import (
	"errors"
	"github.com/pion/webrtc/v4"
)

const maxAudioMessage = 8192 + 24

func (p *Peer) bindAudio(dc *webrtc.DataChannel) {
	p.mu.Lock()
	if p.audio != nil {
		p.mu.Unlock()
		_ = dc.Close()
		return
	}
	p.audio = dc
	p.mu.Unlock()
	p.onMessage(dc, func(m webrtc.DataChannelMessage) {
		if p.FilesOnly() || len(m.Data) <= 24 || len(m.Data) > maxAudioMessage {
			return
		}
		packet := append([]byte(nil), m.Data...)
		select {
		case p.audioInbox <- packet:
			return
		default:
		}
		select {
		case <-p.audioInbox:
		default:
		}
		select {
		case p.audioInbox <- packet:
		default:
		}
	})
}
func (p *Peer) AudioMessages() <-chan []byte { return p.audioInbox }
func (p *Peer) SendAudio(data []byte) error {
	if p.FilesOnly() || len(data) <= 24 || len(data) > maxAudioMessage {
		return errors.New("聲音封包不可傳送")
	}
	p.audioWriteMu.Lock()
	defer p.audioWriteMu.Unlock()
	p.mu.RLock()
	dc, control, screen, closed := p.audio, p.control, p.screen, p.closed
	p.mu.RUnlock()
	if closed || dc == nil || dc.ReadyState() != webrtc.DataChannelStateOpen {
		return errors.New("聲音通道尚未就緒")
	}
	// 零星鍵鼠與心跳不得餓死聲音；只有控制積壓時才讓出傳輸額度。
	if dc.BufferedAmount()+uint64(len(data)) > uint64(4*len(data)) || (control != nil && control.BufferedAmount() > 8*1024) || (screen != nil && screen.BufferedAmount() > 256*1024) {
		return nil
	}
	return p.sendData(dc, data)
}
