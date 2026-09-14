package p2p

import (
	"time"

	"github.com/pion/webrtc/v4"
)

type LayerProgress struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Unit     string `json:"unit"`
	Sent     uint64 `json:"sent"`
	Received uint64 `json:"received"`
	TXIdleMS int64  `json:"txIdleMs"`
	RXIdleMS int64  `json:"rxIdleMs"`
	State    string `json:"state,omitempty"`
	Observed bool   `json:"observed"`
}

func (p *Peer) packetLayers(attempts, verified uint64) []LayerProgress {
	observation, observed := p.observeTransport()
	rows := []LayerProgress{
		{ID: "L1", Name: "測試請求／回覆", Unit: "次", Sent: attempts, Received: verified, Observed: true},
		{ID: "L2", Name: "DataChannel（全部通道）", Unit: "bytes"},
		{ID: "L3", Name: "SCTP", Unit: "bytes"},
		{ID: "L4", Name: "DTLS", State: observation.DTLS},
		{ID: "L5", Name: "ICE 傳輸", Unit: "bytes"},
		{ID: "L6", Name: "UDP socket（本程序）", Unit: "bytes", Sent: diagnosticUDPSent.Load(), Received: diagnosticUDPReceived.Load(), Observed: diagnosticSocketSequence.Load() > 0},
	}
	if !observed {
		return rows
	}
	for _, stat := range observation.Report {
		switch s := stat.(type) {
		case webrtc.DataChannelStats:
			rows[1].Sent += s.BytesSent
			rows[1].Received += s.BytesReceived
			rows[1].Observed = true
		case webrtc.SCTPTransportStats:
			rows[2].Sent = s.BytesSent
			rows[2].Received = s.BytesReceived
			rows[2].Observed = true
		case webrtc.TransportStats:
			rows[4].Sent = s.BytesSent
			rows[4].Received = s.BytesReceived
			rows[4].Observed = true
		}
	}
	return rows
}

type layerPoint struct {
	first, last LayerProgress
	txAt, rxAt  time.Time
}
type layerTracker struct{ rows []layerPoint }

func newLayerTracker() *layerTracker { return &layerTracker{} }
func (t *layerTracker) update(rows []LayerProgress) {
	now := time.Now()
	if len(t.rows) == 0 {
		for _, r := range rows {
			t.rows = append(t.rows, layerPoint{r, r, now, now})
		}
		return
	}
	for i, r := range rows {
		if !r.Observed && r.State == "" {
			continue
		}
		p := &t.rows[i]
		if r.Observed && !p.last.Observed {
			// 第一筆有效取樣才建立基準，不把取樣前的累計流量算成本批進展。
			*p = layerPoint{r, r, now, now}
			continue
		}
		if r.Sent != p.last.Sent {
			p.txAt = now
		}
		if r.Received != p.last.Received {
			p.rxAt = now
		}
		p.last = r
	}
}
func (t *layerTracker) result() []LayerProgress {
	now := time.Now()
	out := make([]LayerProgress, 0, len(t.rows))
	for _, p := range t.rows {
		r := p.last
		if r.Sent >= p.first.Sent {
			r.Sent -= p.first.Sent
		} else {
			r.Sent = 0
		}
		if r.Received >= p.first.Received {
			r.Received -= p.first.Received
		} else {
			r.Received = 0
		}
		r.TXIdleMS = now.Sub(p.txAt).Milliseconds()
		r.RXIdleMS = now.Sub(p.rxAt).Milliseconds()
		out = append(out, r)
	}
	return out
}
