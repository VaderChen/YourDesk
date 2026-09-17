package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
	"yourdesk/internal/clipboard"
	"yourdesk/internal/p2p"
	"yourdesk/internal/peertransport"
	"yourdesk/internal/signaling"
	"yourdesk/internal/streamconfig"
	"yourdesk/internal/streampipeline"
	"yourdesk/internal/video"
)

// 一次連線擁有獨立解碼器、剪貼簿與取消範圍；重連前等待舊資源結束。
type viewerConnection struct {
	peer      *p2p.Peer
	signal    *signaling.Client
	clipboard *clipboard.Sync
	cancel    context.CancelFunc
	cleanup   func()
	once      sync.Once
	readyOnce sync.Once
	ready     chan struct{}
	done      chan struct{}
	workers   sync.WaitGroup
}

func (c *viewerConnection) Close() {
	c.once.Do(func() {
		c.cancel()
		go func() {
			defer close(c.done)
			if c.peer != nil {
				_ = c.peer.Close()
			}
			if c.signal != nil {
				_ = c.signal.Close()
			}
			c.workers.Wait()
			if c.cleanup != nil {
				c.cleanup()
			}
		}()
	})
}

func openViewerConnection(parent, handshakeCtx context.Context, sig *signaling.Client, g *game, codec string, mode peertransport.Mode, silent bool) (*viewerConnection, error) {
	ctx, cancel := context.WithCancel(parent)
	connection := &viewerConnection{signal: sig, clipboard: clipboard.New(), cancel: cancel, ready: make(chan struct{}), done: make(chan struct{})}
	jpegDecoder, selection := video.NewJPEGDecoderForCodec(codec)
	slog.Info("video decoder selected", "backend", selection.Backend, "codec", selection.Codec)
	var videoDecodeFailed atomic.Bool
	var videoDecoder video.DecodeSession
	lastDecoderBackend := ""
	var codecStatusMu sync.Mutex
	sourceModes := map[byte]string{}
	receivedCodec := ""
	var receivedWire byte
	receivedMode := "unknown"

	var stats viewerPipelineStats
	var lastRecoveryRequest atomic.Int64
	var lastVideoSequence uint64
	var lastVideoDisplay int
	decodeFrames := streampipeline.NewConsumer(ctx, func(f p2p.Frame) {
		g.mu.RLock()
		ignore := ctx.Err() != nil || g.agentViewChanging || g.agentPaused || f.ViewID != g.agentViewID
		g.mu.RUnlock()
		if ignore {
			return
		}
		decodeStarted := time.Now()
		defer func() { stats.decodeNanos.Add(uint64(time.Since(decodeStarted))); stats.attempts.Add(1) }()
		var img image.Image
		var e error
		if f.Codec == 0 {
			videoDecoder.Close()
			lastVideoSequence = 0
			img, e = jpegDecoder.Decode(f.JPEG)
		} else {
			if lastVideoSequence != 0 && (f.Sequence != lastVideoSequence+1 || int(f.Display) != lastVideoDisplay) {
				videoDecoder.Close()
				stats.gaps.Add(1)
			}
			lastVideoSequence = f.Sequence
			lastVideoDisplay = int(f.Display)
			img, e = videoDecoder.Decode(video.WireCodec(f.Codec), f.JPEG)
			keyframe, _ := video.IsKeyframe(video.WireCodec(f.Codec), f.JPEG)
			if errors.Is(e, video.ErrNeedKeyframe) || (e != nil && !keyframe) {
				stats.waiting.Add(1)
				now := time.Now().UnixMilli()
				previous := lastRecoveryRequest.Load()
				if now-previous >= 2000 && lastRecoveryRequest.CompareAndSwap(previous, now) {
					g.mu.RLock()
					remote := g.peer
					g.mu.RUnlock()
					if remote != nil && remote.SupportsCommand("video.keyframe") {
						go func() {
							requestCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
							defer cancel()
							_, _ = remote.CallCommand(requestCtx, "video.keyframe")
						}()
					}
				}
				return
			}
			if e == nil {
				backend := videoDecoder.Backend()
				if backend != lastDecoderBackend {
					lastDecoderBackend = backend
					slog.Info("遠端顯示 實際影片解碼後端", "backend", backend)
				}
			}
			if e != nil {
				videoDecodeFailed.Store(true)
			}
		}
		if e != nil {
			stats.failed.Add(1)
			slog.Warn("decode frame", "error", e)
			return
		}
		stats.decoded.Add(1)
		mode := "unknown"
		if f.Codec == 0 {
			mode = "software"
			if jpegDecoder.Hardware() {
				mode = "hardware"
			}
		} else {
			mode = videoDecoder.DecodingMode()
		}
		codecStatusMu.Lock()
		receivedWire = f.Codec
		receivedCodec = map[byte]string{0: "JPEG", 1: "H.264", 2: "HEVC", 3: "AV1"}[f.Codec]
		receivedMode = mode
		codecStatusMu.Unlock()
		g.mu.Lock()
		if ctx.Err() != nil || g.agentViewChanging || g.agentPaused || f.ViewID != g.agentViewID {
			g.mu.Unlock()
			return
		}
		if !f.Keyframe && (g.frame == nil || g.frame.Bounds().Dx() != int(f.Width) || g.frame.Bounds().Dy() != int(f.Height)) {
			g.mu.Unlock()
			return
		}
		if g.displayKnown && (f.Display != g.display || (g.displayPending && !f.Keyframe)) {
			g.mu.Unlock()
			return
		}
		if f.Keyframe {
			g.displayPending = false
		}
		var changed bool
		g.frame, changed = compositeDecodedFrame(g.frame, img, image.Rect(0, 0, int(f.Width), int(f.Height)), int(f.X), int(f.Y), f.Keyframe)
		g.dirty = g.dirty || changed || g.frameDisplay != f.Display || g.frameViewID != f.ViewID
		g.frameViewID = f.ViewID
		g.frameDisplay = f.Display
		g.width, g.height = int(f.Width), int(f.Height)
		g.mu.Unlock()
		connection.readyOnce.Do(func() { close(connection.ready) })
	})
	connection.cleanup = func() { decodeFrames.Close(); videoDecoder.Close(); jpegDecoder.Close() }
	peer, err := p2p.NewViewerWithTransport(handshakeCtx, sig, mode, func(f p2p.Frame) {
		stats.received.Add(1)
		stats.wire.Store(uint32(f.Codec))
		// 封包已重組為獨立記憶體，交給解碼 worker 後即可接收下一幀。
		// 網路回呼不可等待解碼 worker；滿載時略過影格，讓
		// DataChannel read loop 持續消費 SCTP。
		if !decodeFrames.TrySubmit(f) {
			slog.Debug("遠端顯示 解碼佇列滿載，略過影格")
		}
	}, func(c p2p.Control) {
		if ctx.Err() == nil {
			g.receiveDisplays(c)
		}
	}, func(c p2p.Control) {
		if ctx.Err() != nil {
			return
		}
		if c.Type == "desktop-unavailable" {
			if !silent {
				fatal(fmt.Errorf("此裝置沒有桌面環境，請改用命令列連線。"))
			}
			cancel()
			return
		}
		if connection.clipboard.Handle(c) {
			return
		}
		if c.Type == "video-status" {
			g.remoteEnhancement.Store(c.EnhancementReport)
			if c.VideoCodec <= byte(video.WireAV1) && (c.EncodingMode == "hardware" || c.EncodingMode == "software") {
				codecStatusMu.Lock()
				sourceModes[c.VideoCodec] = c.EncodingMode
				codecStatusMu.Unlock()
			}
			return
		}
		if c.Type == "stream-config-result" {
			if c.StreamResult != nil {
				g.streamResult.Store(c.StreamResult)
			}
			return
		}
		if c.Type == "keyboard-capabilities" {
			g.remoteVersion.Store(c.AppVersion)
			if cap := c.StreamCapabilities; cap != nil && cap.SchemaVersion == streamconfig.Version && cap.SessionID != "" {
				g.streamCapabilities.Store(cap)
			}
			g.rawSupported.Store(true)
			g.enhancementSupported.Store(c.EnhancementSupported)
			return
		}
	})
	if err != nil {
		connection.Close()
		return connection, err
	}
	connection.peer = peer
	// 定期公告可接收格式，避免初次 DataChannel 開啟時序遺失協商。
	connection.workers.Add(1)
	go func() {
		defer connection.workers.Done()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		// 背景每秒取樣，避免統計收集阻塞繪圖；以實際間隔計算速度。
		lastSent, lastReceived := peer.TrafficBytes()
		lastSample := time.Now()
		diagnostic := newPipelineSample(&stats, g)
		_, diagnostic.bytes = peer.TrafficBytes()
		lastFrames := g.presentedFrames.Load()
		lastRendered := g.renderedFrames.Load()
		for {

			diagnostic.report(&stats, g, peer)
			now := time.Now()
			if elapsed := now.Sub(lastSample).Seconds(); elapsed >= 0.5 {
				sent, received := peer.TrafficBytes()
				tx, rx := 0.0, 0.0
				if sent >= lastSent {
					tx = float64(sent-lastSent) / elapsed
				}
				if received >= lastReceived {
					rx = float64(received-lastReceived) / elapsed
				}
				frames := g.presentedFrames.Load()
				rendered := g.renderedFrames.Load()
				renderMode := viewerInterpolation.Load()
				fps := float64(frames-lastFrames) / elapsed
				if renderMode {
					fps = float64(rendered-lastRendered) / elapsed
				}
				nativeSetTraffic(tx, rx, fps, renderMode)
				lastRendered = rendered
				codecStatusMu.Lock()
				nativeSetCodecStatus(receivedCodec, sourceEncodingMode(receivedWire, sourceModes[receivedWire]), receivedMode)
				codecStatusMu.Unlock()
				lastFrames = frames
				lastSent, lastReceived, lastSample = sent, received, now
			}
			var advertised []byte
			if codec != "software" && codec != "software-jpeg" {
				advertised = video.ReceiverCodecs()
			}
			if videoDecodeFailed.Load() {
				advertised = nil
			}
			_ = peer.SendControl(p2p.Control{Type: "video-capabilities", Codecs: advertised, HardwareDecodeCodecs: video.ReceiverHardwareCodecs(advertised), KeyframeInterval: 10})
			select {
			case <-ctx.Done():
				return
			case <-peer.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	connection.workers.Add(1)
	go func() { defer connection.workers.Done(); connection.clipboard.Run(ctx, peer) }()
	connection.workers.Add(1)
	go func() {
		defer connection.workers.Done()
		select {
		case <-ctx.Done():
		case <-peer.Done():
		}
		connection.Close()
	}()
	return connection, nil
}
