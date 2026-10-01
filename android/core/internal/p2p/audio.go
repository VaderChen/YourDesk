package p2p

import "github.com/pion/webrtc/v4"

// 聲音與畫面分流；只保留最近八包，接收回呼不等待解碼或播放。
func (p *Peer) bindAudio(dc *webrtc.DataChannel) {
	p.mu.Lock()
	if p.audio != nil || p.closed {
		p.mu.Unlock()
		_ = dc.Close()
		return
	}
	p.audio = dc
	p.mu.Unlock()
	p.onMessage(dc, func(m webrtc.DataChannelMessage) { p.enqueueAudio(m.Data) })
}

func (p *Peer) enqueueAudio(data []byte) {
	if len(data) <= 24 || len(data) > 8192+24 {
		return
	}
	packet := append([]byte(nil), data...)
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
}

func (p *Peer) ReadAudioPacket() []byte {
	select {
	case packet := <-p.audioInbox:
		return packet
	default:
		return nil
	}
}
