package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"
	"yourdesk/internal/diagnostics"
	"yourdesk/internal/p2p"
	"yourdesk/internal/peertransport"
	"yourdesk/internal/signaling"
	"yourdesk/internal/streamconfig"
	"yourdesk/internal/streampipeline"
	"yourdesk/internal/video"
)

// 背景診斷只驗證串流及解碼，不建立 遠端顯示、不注入輸入、不同步剪貼簿。
func runBackgroundDiagnostic(url, room, codec string, mode peertransport.Mode) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	passwords := newPasswordInput()
	secret, err := passwords.read(ctx, false)
	if err != nil {
		return err
	}
	var sig *signaling.Client
	if address, direct := signaling.DirectAddress(room); direct {
		sig, err = signaling.DialDirect(ctx, address, secret)
	} else {
		sig, err = signaling.Dial(ctx, url, room, signaling.RoleViewer, secret)
	}
	if err != nil {
		return err
	}
	defer sig.Close()
	sig.ResolveSecret = passwords.resolver()
	refreshViewerLanguage("")
	jpeg, _ := video.NewJPEGDecoderForCodec(codec)
	defer jpeg.Close()
	var decoder video.DecodeSession
	defer decoder.Close()
	var stats viewerPipelineStats
	var capabilities atomic.Pointer[streamconfig.Capabilities]
	var applied atomic.Pointer[streamconfig.Result]
	var failed atomic.Bool
	var remoteVersion atomic.Value
	var previous uint64
	var display int
	first := make(chan struct{}, 1)
	worker := streampipeline.NewConsumer(ctx, func(f p2p.Frame) {
		started := time.Now()
		defer func() { stats.decodeNanos.Add(uint64(time.Since(started))); stats.attempts.Add(1) }()
		var err error
		if f.Codec == 0 {
			decoder.Close()
			previous = 0
			_, err = jpeg.Decode(f.JPEG)
		} else {
			if previous != 0 && (f.Sequence != previous+1 || f.Display != display) {
				decoder.Close()
				stats.gaps.Add(1)
			}
			previous, display = f.Sequence, f.Display
			_, err = decoder.Decode(video.WireCodec(f.Codec), f.JPEG)
			key, _ := video.IsKeyframe(video.WireCodec(f.Codec), f.JPEG)
			if errors.Is(err, video.ErrNeedKeyframe) || (err != nil && !key) {
				stats.waiting.Add(1)
				return
			}
			if err != nil {
				failed.Store(true)
			}
		}
		if err != nil {
			stats.failed.Add(1)
			return
		}
		stats.decoded.Add(1)
		select {
		case first <- struct{}{}:
		default:
		}
	})
	defer worker.Close()
	peer, err := p2p.NewViewerWithTransport(ctx, sig, mode, func(f p2p.Frame) { stats.received.Add(1); stats.wire.Store(uint32(f.Codec)); worker.Submit(f) }, func(c p2p.Control) {
		if c.Type == "desktop-unavailable" {
			fatal(fmt.Errorf("此裝置沒有桌面環境，請改用命令列連線。"))
			return
		}
		if c.Type == "keyboard-capabilities" {
			remoteVersion.Store(c.AppVersion)
		}
		if c.Type == "keyboard-capabilities" && c.StreamCapabilities != nil {
			capabilities.Store(c.StreamCapabilities)
		}
		if c.Type == "stream-config-result" && c.StreamResult != nil {
			applied.Store(c.StreamResult)
		}
	})
	if err != nil {
		return err
	}
	defer func() { worker.Close(); peer.Close() }()
	emitUIEvent("authenticated", "")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	life := time.NewTimer(45 * time.Second)
	defer life.Stop()
	var connected bool
	at := time.Now()
	_, lastBytes := peer.TrafficBytes()
	var lastReceived, lastDecoded, lastErrors, lastGaps, lastNanos, lastAttempts uint64
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-peer.Done():
			return fmt.Errorf("背景診斷連線已中斷")
		case <-life.C:
			return nil
		case <-first:
			if !connected {
				connected = true
				emitUIEvent("frame", "")
			}
		case <-ticker.C:
			var advertised []byte
			if codec != "software" && codec != "software-jpeg" {
				advertised = video.ReceiverCodecs()
			}
			if failed.Load() {
				advertised = nil
			}
			_ = peer.SendControl(p2p.Control{Type: "video-capabilities", Codecs: advertised, HardwareDecodeCodecs: video.ReceiverHardwareCodecs(advertised), KeyframeInterval: 10})
			if cap := capabilities.Load(); cap != nil {
				request := streamconfig.Compose("standard", 8192, 8192, false)
				applySourceFPSLimit(&request, cap)
				applyKeyframeInterval(&request, cap)
				request.SessionID = cap.SessionID
				request.Revision = 1
				_ = peer.SendControl(p2p.Control{Type: "stream-config", StreamConfig: &request})
			}
			now := time.Now()
			elapsed := now.Sub(at).Seconds()
			if elapsed < 5 {
				continue
			}
			_, receivedBytes := peer.TrafficBytes()
			received, decoded, failures, gaps, nanos, attempts := stats.received.Load(), stats.decoded.Load(), stats.failed.Load(), stats.gaps.Load(), stats.decodeNanos.Load(), stats.attempts.Load()
			sample := diagnostics.Sample{Transport: peer.TransportMode(), Background: true, StartedAt: at.UnixMilli(), Seconds: elapsed, RTTMS: peer.RoundTripMS(), ReceiveMbps: float64(receivedBytes-lastBytes) * 8 / elapsed / 1e6, DecodedPerSec: float64(decoded-lastDecoded) / elapsed, ReceivedPerSec: float64(received-lastReceived) / elapsed, Received: received - lastReceived, Errors: failures - lastErrors, Gaps: gaps - lastGaps, Codec: map[uint32]string{0: "JPEG（可能為區塊）", 1: "H.264", 2: "HEVC", 3: "AV1"}[stats.wire.Load()]}
			if version, ok := remoteVersion.Load().(string); ok {
				sample.RemoteVersion = version
			}
			if attempts > lastAttempts {
				sample.CallbackMS = float64(nanos-lastNanos) / float64(attempts-lastAttempts) / 1e6
			}
			if result := applied.Load(); result != nil && result.Accepted {
				sample.FPSLimit = result.Effective.FPS
			}
			data, _ := json.Marshal(struct {
				Event  string             `json:"event"`
				Sample diagnostics.Sample `json:"sample"`
			}{"diagnostic-sample", sample})
			fmt.Fprintln(os.Stdout, uiEventPrefix+string(data))
			at, lastBytes = now, receivedBytes
			lastReceived, lastDecoded, lastErrors, lastGaps, lastNanos, lastAttempts = received, decoded, failures, gaps, nanos, attempts
		}
	}
}
