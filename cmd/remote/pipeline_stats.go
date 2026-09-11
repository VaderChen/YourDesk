package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"time"
	"yourdesk/internal/diagnostics"
	"yourdesk/internal/p2p"
)

// 收到的單位在 JPEG 模式可能是區塊，不能一律解讀為來源 FPS。
type viewerPipelineStats struct {
	wire                                                            atomic.Uint32
	received, decoded, failed, waiting, gaps, decodeNanos, attempts atomic.Uint64
}
type pipelineSample struct {
	at                                                                             time.Time
	bytes                                                                          uint64
	received, decoded, failed, waiting, gaps, nanos, attempts, presented, rendered uint64
}

func newPipelineSample(s *viewerPipelineStats, g *game) pipelineSample {
	return pipelineSample{time.Now(), 0, s.received.Load(), s.decoded.Load(), s.failed.Load(), s.waiting.Load(), s.gaps.Load(), s.decodeNanos.Load(), s.attempts.Load(), g.presentedFrames.Load(), g.renderedFrames.Load()}
}
func (p *pipelineSample) report(s *viewerPipelineStats, g *game, peer *p2p.Peer) {
	if time.Since(p.at) < 5*time.Second {
		return
	}
	next := newPipelineSample(s, g)
	elapsed := next.at.Sub(p.at).Seconds()
	_, next.bytes = peer.TrafficBytes()
	limit, gop := 0, 0
	accepted := false
	if r := g.streamResult.Load(); r != nil {
		limit = r.Effective.FPS
		gop = r.Effective.KeyframeInterval
		accepted = r.Accepted
	}
	codec := map[uint32]string{0: "JPEG（可能為區塊）", 1: "H.264", 2: "HEVC"}[s.wire.Load()]
	averageMS := 0.0
	if count := next.attempts - p.attempts; count > 0 {
		averageMS = float64(next.nanos-p.nanos) / float64(count) / 1e6
	}
	slog.Info("遠端顯示 管線統計", "codec", codec, "remoteConfigAccepted", accepted, "remoteFPSLimit", limit, "remoteGOP", gop, "receivedUnitsPerSec", float64(next.received-p.received)/elapsed, "decodedUnitsPerSec", float64(next.decoded-p.decoded)/elapsed, "sourcePresentedFPS", float64(next.presented-p.presented)/elapsed, "renderFPS", float64(next.rendered-p.rendered)/elapsed, "callbackAverageMs", averageMS, "sequenceGaps", next.gaps-p.gaps, "waitingForIDR", next.waiting-p.waiting, "decodeErrors", next.failed-p.failed)
	if g.uiEvents {
		rate := 0.0
		if p.bytes > 0 && next.bytes >= p.bytes {
			rate = float64(next.bytes-p.bytes) * 8 / elapsed / 1e6
		}
		sample := diagnostics.Sample{StartedAt: p.at.UnixMilli(), Seconds: elapsed, RTTMS: peer.RoundTripMS(), ReceiveMbps: rate, SourceFPS: float64(next.presented-p.presented) / elapsed, RenderFPS: float64(next.rendered-p.rendered) / elapsed, ReceivedPerSec: float64(next.received-p.received) / elapsed, CallbackMS: averageMS, FPSLimit: limit, Codec: codec, Gaps: next.gaps - p.gaps, Errors: next.failed - p.failed, Received: next.received - p.received}
		if version, ok := g.remoteVersion.Load().(string); ok {
			sample.RemoteVersion = version
		}
		data, err := json.Marshal(struct {
			Event  string             `json:"event"`
			Sample diagnostics.Sample `json:"sample"`
		}{"diagnostic-sample", sample})
		if err == nil {
			fmt.Fprintln(os.Stdout, uiEventPrefix+string(data))
		}
	}
	*p = next
}
