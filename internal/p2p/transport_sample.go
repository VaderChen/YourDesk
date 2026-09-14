package p2p

import (
	"context"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
)

type channelObservation struct {
	Label, State string
	Buffered     uint64
}

type transportObservation struct {
	Report   webrtc.StatsReport
	DTLS     string
	Channels []channelObservation
}

type transportSampleRequest struct {
	done  chan struct{}
	value transportObservation
	at    time.Time
}

type transportSampler struct {
	mu      sync.Mutex
	pending *transportSampleRequest
}

// 每個 Peer 至多一個統計取樣工作；逾時不會持續堆出新的 GetStats goroutine。
// 完成的結果最多重用 250 ms，取樣不持有收送端使用的診斷鎖。
func (s *transportSampler) get(ctx context.Context, stopped <-chan struct{}, collect func() transportObservation) (transportObservation, bool) {
	select {
	case <-ctx.Done():
		return transportObservation{}, false
	case <-stopped:
		return transportObservation{}, false
	default:
	}
	s.mu.Lock()
	request := s.pending
	if request != nil {
		select {
		case <-request.done:
			if time.Since(request.at) >= 250*time.Millisecond {
				request = nil
			}
		default:
		}
	}
	if request == nil {
		request = &transportSampleRequest{done: make(chan struct{})}
		s.pending = request
		go func(r *transportSampleRequest) {
			r.value = collect()
			r.at = time.Now()
			close(r.done)
		}(request)
	}
	s.mu.Unlock()
	select {
	case <-ctx.Done():
		return transportObservation{}, false
	case <-stopped:
		return transportObservation{}, false
	case <-request.done:
		return request.value, true
	}
}

func (p *Peer) observeTransport() (transportObservation, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	return p.transportSamples.get(ctx, p.Done(), func() transportObservation {
		out := transportObservation{Report: p.pc.GetStats()}
		if sctp := p.pc.SCTP(); sctp != nil && sctp.Transport() != nil {
			out.DTLS = sctp.Transport().State().String()
		}
		p.mu.RLock()
		channels := []*webrtc.DataChannel{p.screen, p.control, p.clipboard}
		p.mu.RUnlock()
		for _, dc := range channels {
			if dc != nil {
				out.Channels = append(out.Channels, channelObservation{dc.Label(), dc.ReadyState().String(), dc.BufferedAmount()})
			}
		}
		return out
	})
}
