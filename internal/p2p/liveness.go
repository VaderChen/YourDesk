package p2p

import (
	"context"
	"log/slog"
	"time"
	"yourdesk/internal/authlog"

	"github.com/pion/webrtc/v4"
)

// ICE 仍存活不代表 SCTP／控制命令仍可往返。舊版未公告 ping 時不啟用。
func (p *Peer) startLiveness() {
	p.livenessOnce.Do(func() {
		{
			go func() {
				ticker := time.NewTicker(10 * time.Second)
				defer ticker.Stop()
				previous := p.trafficSnapshot()
				for {
					select {
					case <-p.Done():
						authlog.Event("peer_closed", nil)
						return
					case <-ticker.C:
						current := p.trafficSnapshot()
						if authlog.IsEnabled() {
							authlog.Event("application-traffic", current.since(previous))
						}
						previous = current
						p.diagnosticTransport()
					}
				}
			}()
		}
		go func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go func() {
				select {
				case <-p.Done():
					cancel()
				case <-ctx.Done():
				}
			}()
			_, previousReceived := p.TrafficBytes()
			monitorLiveness(ctx, func() bool { return p.Connected() && p.SupportsCommand("ping") }, func(ctx context.Context) error {
				_, err := p.CallCommand(ctx, "ping")
				if err != nil && ctx.Err() != context.Canceled {
					authlog.Event("ping_timeout", nil)
					p.diagnosticTransport()
					authlog.Stacks()
				}
				return err
			}, func() {
				slog.Error("P2P 控制通道超過恢復寬限仍無回應，結束失效連線")
				authlog.Event("heartbeat-failed", nil)
				authlog.Stacks()
				_ = p.Close()
			}, func() bool {
				_, received := p.TrafficBytes()
				progress := received > previousReceived
				previousReceived = received
				return progress
			})
		}()
	})
}

func monitorLiveness(ctx context.Context, ready func() bool, ping func(context.Context) error, failed func(), progress ...func() bool) {
	ctx, cancelMonitor := context.WithCancel(ctx)
	defer cancelMonitor()
	ping = boundedLivenessPing(ctx, ping)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	misses := 0
	var firstFailure, lastProgress time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if !ready() {
			misses = 0
			firstFailure = time.Time{}
			lastProgress = time.Time{}
			continue
		}
		requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := ping(requestCtx)
		cancel()
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			misses = 0
			firstFailure = time.Time{}
			lastProgress = time.Time{}
			continue
		}
		misses++
		now := time.Now()
		if firstFailure.IsZero() {
			firstFailure = now
			lastProgress = now
		}
		if len(progress) > 0 && progress[0]() {
			lastProgress = now
		}
		slog.Warn("P2P 存活確認失敗", "consecutive", misses, "error", err)
		// 零星資料不能永遠掩蓋無法往返的控制通道。
		if now.Sub(firstFailure) >= 90*time.Second || (now.Sub(firstFailure) >= 30*time.Second && now.Sub(lastProgress) >= 30*time.Second) {
			failed()
			return
		}
	}
}

// diagnosticTransport 不寫入端點、命令內容或媒體資料。
func (p *Peer) diagnosticTransport() {
	if !authlog.IsEnabled() {
		return
	}
	observation, ok := p.observeTransport()
	if !ok {
		authlog.Event("transport-sample-unavailable", nil)
		return
	}
	authlog.Event("dtls", map[string]any{"layer": "L4", "state": observation.DTLS})
	for _, stat := range observation.Report {
		switch s := stat.(type) {
		case webrtc.DataChannelStats:
			authlog.Event("data-channel", map[string]any{"layer": "L2", "label": s.Label, "sent": s.BytesSent, "received": s.BytesReceived, "messagesSent": s.MessagesSent, "messagesReceived": s.MessagesReceived})
		case webrtc.TransportStats:
			authlog.Event("transport", map[string]any{"layer": "L5", "sent": s.BytesSent, "received": s.BytesReceived})
		case webrtc.SCTPTransportStats:
			authlog.Event("sctp", map[string]any{"layer": "L3", "sent": s.BytesSent, "received": s.BytesReceived, "receiverWindow": s.ReceiverWindow, "congestionWindow": s.CongestionWindow, "rtt": s.SmoothedRoundTripTime})
		}
	}
	for _, channel := range observation.Channels {
		authlog.Event("channel", map[string]any{"layer": "L2", "label": channel.Label, "state": channel.State, "buffered": channel.Buffered})
	}
}
