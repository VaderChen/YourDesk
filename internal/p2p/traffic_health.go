package p2p

import "time"

type trafficSnapshot struct {
	at                       time.Time
	sent, received, messages uint64
}

func (p *Peer) trafficSnapshot() trafficSnapshot {
	return trafficSnapshot{time.Now(), p.sentBytes.Load(), p.receivedBytes.Load(), p.receivedMessages.Load()}
}

// 計數的是 DataChannel 應用訊息（影像分片亦算一則），不是 UDP 封包。
// 每秒不到 0.2 則僅標示低接收流量，不作為斷線條件。
func (s trafficSnapshot) since(previous trafficSnapshot) map[string]any {
	seconds := s.at.Sub(previous.at).Seconds()
	if seconds <= 0 {
		seconds = 1
	}
	messages := s.messages - previous.messages
	state := "active"
	if messages == 0 {
		state = "no-receive"
	} else if float64(messages)/seconds <= 0.2 {
		state = "low-receive"
	}
	return map[string]any{"state": state, "seconds": seconds, "sentBytes": s.sent - previous.sent, "receivedBytes": s.received - previous.received, "receivedMessages": messages}
}
