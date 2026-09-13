// Package hostsession 共用一般桌面與精簡救援環境的遠端工作階段。
package hostsession

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"image"
	"io"
	"log/slog"
	"math"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"yourdesk/internal/agentvideo"
	"yourdesk/internal/clipboard"
	"yourdesk/internal/desktop"
	"yourdesk/internal/input"
	"yourdesk/internal/optimization"
	"yourdesk/internal/p2p"
	"yourdesk/internal/peertransport"
	"yourdesk/internal/rawkey"
	"yourdesk/internal/remotedata"
	"yourdesk/internal/signaling"
	"yourdesk/internal/streamconfig"
	"yourdesk/internal/streampipeline"
	"yourdesk/internal/terminal"
	"yourdesk/internal/video"
)

var incomingGeneration atomic.Uint64
var incomingPeer atomic.Pointer[p2p.Peer]
var activeSession atomic.Bool

type Options struct {
	Headless              bool
	Version               string
	DisableClipboard      bool
	DisableRemoteData     bool
	PrimaryDisplayOnly    bool
	OnState               func(string)
	Transport             peertransport.Mode
	Display, FPS, Quality int
	CodecGoal             optimization.Goal
	Codec                 string
}

func Stream(ctx context.Context, sig *signaling.Client, options Options) error {
	if options.Headless {
		return commandSession(ctx, sig, options)
	}
	switch video.Codec(options.Codec) {
	case video.CodecSoftwareAV1, video.CodecHardwareAV1, video.CodecAuto, video.CodecHardwareH264, video.CodecHardwareHEVC, video.CodecSoftwareH264, video.CodecHardwareJPEG, video.CodecSoftwareJPEG:
	default:
		return fmt.Errorf("不支援的影像編碼：%s", options.Codec)
	}
	// JPEG 保留作為舊 遠端顯示 與硬體失敗時的相容路徑。
	jpegEncoder, jpegSelection := video.NewJPEGEncoderForCodec(video.Codec(options.Codec))
	defer jpegEncoder.Close()
	slog.Info("JPEG 相容編碼器", "selected", jpegSelection.Selected)
	var remoteCodecs atomic.Uint32
	var remoteHardwareCodecs atomic.Uint32
	var hardwareEncoder video.IntraEncoder
	var activeCodec video.WireCodec
	var activeEncoderCodec video.Codec
	failedCodecs := make(map[codecAttempt]bool)
	defer func() {
		if hardwareEncoder != nil {
			hardwareEncoder.Close()
		}
	}()
	capturer := desktop.NewLiveCapturer()
	defer capturer.Close()
	selected := options.Display
	var displayMu sync.Mutex
	agentView := newAgentVideo()
	var captureEpoch atomic.Uint64
	var inputBounds image.Rectangle
	buttons := make(map[int]bool)
	keys := make(map[string]bool)
	legacyTargets := make(map[string]string)
	rawHeld := make(map[string]rawkey.Event)
	releaseRaw := func() {
		for id, event := range rawHeld {
			event.Down = false
			event.Repeat = false
			event.Modifiers = 0
			_ = (input.Native{}).RawKey(event)
			delete(rawHeld, id)
		}
	}
	nextDisplay := make(chan p2p.Control, 1)
	profileRequests := make(chan streamconfig.Request, 1)
	configSession := rand.Text()
	var remoteGOP atomic.Int32
	var configMode atomic.Bool
	var configRevision atomic.Uint64
	var configResult atomic.Pointer[streamconfig.Result]
	var configPeer atomic.Pointer[p2p.Peer]
	sendConfigResult := func(result *streamconfig.Result) {
		if p := configPeer.Load(); p != nil {
			_ = p.SendControl(p2p.Control{Type: "stream-config-result", StreamResult: result})
		}
	}
	var displayRequest uint64
	releaseInput := func() {
		releaseRaw()
		controller := input.Native{Bounds: inputBounds}
		for button, down := range buttons {
			if down {
				_ = controller.Button(button, false)
			}
			delete(buttons, button)
		}
		for key, down := range keys {
			if down {
				target := key
				if mapped, ok := legacyTargets[key]; ok {
					target = mapped
				}
				_ = controller.Key(target, false)
			}
			delete(keys, key)
			delete(legacyTargets, key)
		}
	}

	if err := input.EnsurePermissions(); err != nil {
		return err
	}
	var clipboardSync *clipboard.Sync
	if !options.DisableClipboard {
		clipboardSync = clipboard.New()
	}
	var recovery atomic.Uint64
	var lastKeyframeRequest atomic.Int64
	var fullFrameRequested atomic.Bool
	var terminalActive atomic.Bool
	var authorized atomic.Bool
	peer, err := p2p.NewHostWithTransport(ctx, sig, options.Transport, func(c p2p.Control) {
		if c.Type == "video-capabilities" {
			if c.KeyframeInterval == 10 {
				remoteGOP.Store(10)
			} else {
				remoteGOP.Store(1)
			}
			var mask uint32
			for _, codec := range c.Codecs {
				if codec == byte(video.WireH264) || codec == byte(video.WireHEVC) || codec == byte(video.WireAV1) {
					mask |= 1 << codec
				}
			}
			var hardwareMask uint32
			for _, codec := range c.HardwareDecodeCodecs {
				if codec == byte(video.WireH264) || codec == byte(video.WireHEVC) || codec == byte(video.WireAV1) {
					hardwareMask |= 1 << codec
				}
			}
			remoteHardwareCodecs.Store(hardwareMask & mask)
			remoteCodecs.Store(mask)
			return
		}
		if !authorized.Load() {
			return
		}
		if !options.DisableClipboard && clipboardSync.Handle(c) {
			return
		}
		if c.Type == "raw-key-reset" {
			displayMu.Lock()
			releaseRaw()
			displayMu.Unlock()
			return
		}
		if c.Type == "quality-profile" || c.Type == "stream-config" {
			var request streamconfig.Request
			if c.Type == "stream-config" {
				if c.StreamConfig == nil {
					return
				}
				request = *c.StreamConfig
				err := request.Validate()
				if request.SessionID != configSession || request.Revision == 0 {
					err = fmt.Errorf("串流設定工作階段或序號無效")
				}
				if err != nil {
					sendConfigResult(&streamconfig.Result{SessionID: configSession, Revision: request.Revision, Error: err.Error()})
					return
				}
				if request.Revision <= configRevision.Load() {
					if result := configResult.Load(); result != nil && result.Revision == request.Revision {
						sendConfigResult(result)
					}
					return
				}
				configMode.Store(true)
				configRevision.Store(request.Revision)
			} else {
				if configMode.Load() {
					return
				}
				request = streamconfig.ComposeLegacy(c.Profile, c.ViewWidth, c.ViewHeight, c.ImageEnhancement)
				if request.Validate() != nil {
					return
				}
			}
			select {
			case <-profileRequests:
			default:
			}
			select {
			case profileRequests <- request:
			default:
			}
			return
		}
		if c.Type == "display-next" || c.Type == "display-select" {
			select {
			case nextDisplay <- c:
			default:
			}
			return
		}
		displayMu.Lock()
		defer displayMu.Unlock()
		// 舊畫面送出的輸入不可落到剛切換的新螢幕。
		if (c.Type == "move" || c.Type == "button") && c.ViewID != agentView.view.ViewID {
			return
		}
		if inputBounds.Empty() || (c.Display != nil && *c.Display != selected) {
			return
		}
		if math.IsNaN(c.X) || math.IsNaN(c.Y) || math.IsInf(c.X, 0) || math.IsInf(c.Y, 0) {
			return
		}
		if c.Type == "raw-key" {
			if c.RawKey == nil || c.RawKey.Reset || (c.RawKey.Platform != "darwin" && c.RawKey.Platform != "windows") {
				return
			}
			event := *c.RawKey
			if _, err := rawkey.Translate(event.Platform, event.Code, runtime.GOOS); err != nil {
				slog.Warn("原始按鍵無法映射", "error", err)
				return
			}
			reconcileRawModifiers(event, rawHeld)
			if err := injectRawKey(event, rawHeld); err != nil {
				slog.Warn("原始按鍵注入失敗", "error", err)
				return
			}
			id := fmt.Sprintf("%s/%d", event.Platform, event.Code)
			if event.Down {
				rawHeld[id] = event
			} else {
				delete(rawHeld, id)
			}
			return
		}
		originalKey := c.Key
		if c.Type == "key" {
			c.Key = rawkey.LegacyKey(c.Platform, runtime.GOOS, c.Key, c.DisableMapping)
			if !c.Down {
				if previous, ok := legacyTargets[originalKey]; ok {
					c.Key = previous
				}
			}
			for other, down := range keys {
				if down && other != originalKey && legacyTargets[other] == c.Key {
					keys[originalKey] = c.Down
					legacyTargets[originalKey] = c.Key
					return
				}
			}
			legacyTargets[originalKey] = c.Key
		}
		handleControl(input.Native{Bounds: inputBounds}, c)
		if c.Type == "button" {
			buttons[c.Button] = c.Down
		}
		if c.Type == "key" {
			keys[originalKey] = c.Down
		}
	})
	if err != nil {
		return err
	}
	defer func() {
		authorized.Store(false)
		peer.Close()
		displayMu.Lock()
		defer displayMu.Unlock()
		releaseInput()
	}()
	if !activeSession.CompareAndSwap(false, true) {
		return fmt.Errorf("Client 已有遠端工作階段")
	}
	defer activeSession.Store(false)
	sessionGeneration := incomingGeneration.Add(1)
	incomingPeer.Store(peer)
	defer incomingPeer.CompareAndSwap(peer, nil)
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-peer.Done():
				return
			case <-ticker.C:
				if peer.Connected() {
					if options.OnState != nil {
						options.OnState("connected")
					}
					fmt.Printf("YOURDESK_UI_EVENT {\"event\":\"host-connected\",\"session\":%d}\n", sessionGeneration)
					select {
					case <-ctx.Done():
					case <-peer.Done():
					}
					if options.OnState != nil {
						options.OnState("disconnected")
					}
					fmt.Printf("YOURDESK_UI_EVENT {\"event\":\"host-disconnected\",\"session\":%d}\n", sessionGeneration)
					return
				}
			}
		}
	}()
	configPeer.Store(peer)
	authorized.Store(true)
	if !options.DisableRemoteData {
		remotedata.Register(peer, authorized.Load)
		terminal.Register(peer, authorized.Load, terminalActive.Store)
	}
	_ = peer.RegisterCommand("input.reset", func(commandCtx context.Context) (any, error) {
		displayMu.Lock()
		defer displayMu.Unlock()
		if err := commandCtx.Err(); err != nil {
			return nil, err
		}
		if !authorized.Load() {
			return nil, fmt.Errorf("工作階段已結束")
		}
		releaseInput()
		return map[string]bool{"released": true}, nil
	})
	_ = peer.RegisterCommand("video.keyframe", func(commandCtx context.Context) (any, error) {
		if err := commandCtx.Err(); err != nil {
			return nil, err
		}
		if !authorized.Load() {
			return nil, fmt.Errorf("工作階段已結束")
		}
		now := time.Now().UnixMilli()
		previous := lastKeyframeRequest.Load()
		if now-previous < 1000 || !lastKeyframeRequest.CompareAndSwap(previous, now) {
			return map[string]bool{"accepted": true, "coalesced": true}, nil
		}
		fullFrameRequested.Store(true)
		recovery.Add(1)
		return map[string]bool{"accepted": true}, nil
	})

	agentView.register(peer, func(commandCtx context.Context, request agentvideo.Request) (agentvideo.State, error) {
		displayMu.Lock()
		defer displayMu.Unlock()
		if err := commandCtx.Err(); err != nil {
			return agentvideo.State{}, err
		}
		count := capturer.Count()
		if options.PrimaryDisplayOnly {
			count = min(count, 1)
		}
		if selected < 0 || selected >= count {
			return agentvideo.State{}, fmt.Errorf("目前沒有可擷取的螢幕")
		}
		releaseInput()
		if request.Mode != "" {
			agentView.view.Mode = request.Mode
		}
		if request.FullScreen {
			agentView.view.Region = agentvideo.Full()
		}
		if request.Region != nil {
			agentView.view.Region = *request.Region
		}
		agentView.view.ViewID++
		agentView.view.Display, agentView.view.DisplayCount = selected, count
		inputBounds = agentView.view.Region.Bounds(capturer.Bounds(selected))
		captureEpoch.Add(1)
		recovery.Add(1)
		fullFrameRequested.Store(true)
		return agentView.view, nil
	}, func(commandCtx context.Context, state agentvideo.State) (image.Image, error) {
		if err := commandCtx.Err(); err != nil {
			return nil, err
		}
		displayMu.Lock()
		defer displayMu.Unlock()
		if selected != state.Display || agentView.view.ViewID != state.ViewID {
			return nil, fmt.Errorf("擷取期間視野已變更，請重試")
		}
		// 一律向 OS 取得新畫面，不使用串流快取。
		return (desktop.ScreenshotCapturer{}).Capture(state.Display)
	}, authorized.Load)
	if !options.DisableClipboard {
		go clipboardSync.Run(ctx, peer)
	}
	fmt.Println(`YOURDESK_UI_EVENT {"event":"host-ready"}`)
	fmt.Println(`YOURDESK_UI_EVENT {"event":"authenticated"}`)
	slog.Info("遠端通道配對完成", "transport", peer.TransportMode())
	// 通道建立後才固定策略，Host 等待配對期間仍可接收背景偵測結果。
	policy := optimization.Snapshot()
	slog.Info("套用本機影像策略", "version", policy.Version, "compareBytes", policy.CompareBytes, "verifiedEncoders", len(policy.Encoders))

	baseFPS := max(1, options.FPS)
	streamFPS := baseFPS
	captureTickFPS := baseFPS
	interval := time.Second / time.Duration(streamFPS)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var seq uint64
	delta := desktop.DeltaEncoder{TileSize: desktop.DefaultTileSize, Quality: clamp(options.Quality, 1, 100), Encoder: jpegEncoder, CompareBytes: policy.CompareBytes}
	lastKeyframe := time.Now().Add(-time.Hour)
	var lastDisplayReport time.Time
	lastCount := -1
	profile := "standard"
	enhancement := false
	activeConfig := streamconfig.Compose("standard", 8192, 8192, false)
	transmissionLimit := func() int {
		if activeConfig.Source.Rate.Mode == "cbr" && activeConfig.Source.Rate.TargetBps > 0 {
			return activeConfig.Source.Rate.TargetBps
		}
		// 尚未收到新版 Viewer 設定時也使用預設上限。
		return 12_000_000
	}
	encodeQuality := clamp(options.Quality, 1, 100)
	var streamScaler desktop.StreamScaler
	defer streamScaler.Close()
	var encodedSize image.Point
	lastEncoderBackend := ""
	lastStatus := time.Time{}
	var lastStatusCodec video.WireCodec
	lastStatusMode := ""
	var enhancementReport p2p.EnhancementReport
	configuredGOP := func() int {
		// 只有 遠端顯示 已宣告連續解碼能力時才能使用參考影格。
		if remoteGOP.Load() != 10 {
			return 1
		}
		if activeConfig.Source.KeyframeInterval > 0 {
			return activeConfig.Source.KeyframeInterval
		}
		return 10
	}
	reportEncoding := func(codec video.WireCodec, hardware bool) {
		if activeConfig.Revision > 0 {
			result := &streamconfig.Result{SessionID: configSession, Revision: activeConfig.Revision, Accepted: true, Effective: streamconfig.Effective{Width: enhancementReport.StreamWidth, Height: enhancementReport.StreamHeight, FPS: streamFPS, Bitrate: enhancementReport.Bitrate, Quality: encodeQuality, KeyframeInterval: configuredGOP()}}
			if !hardware || codec == video.WireJPEG {
				result.Effective.Bitrate = 0
				result.Effective.KeyframeInterval = 0
			}
			// 編碼器即使退回 JPEG，傳送階段仍會執行相同流量上限。
			result.Effective.Bitrate = transmissionLimit()
			previous := configResult.Load()
			if previous == nil || *previous != *result {
				configResult.Store(result)
				sendConfigResult(result)
			}
		}
		mode := "software"
		if !hardware {
			enhancementReport.Bitrate = 0
		}
		if hardware {
			mode = "hardware"
		}
		if codec != lastStatusCodec || mode != lastStatusMode || time.Since(lastStatus) >= time.Second {
			if peer.SendControl(p2p.Control{Type: "video-status", VideoCodec: byte(codec), EncodingMode: mode, EnhancementReport: &enhancementReport}) == nil {
				lastStatus = time.Now()
				lastStatusCodec = codec
				lastStatusMode = mode
			}
		}
	}

	var captureFPS atomic.Int32
	captureFPS.Store(int32(streamFPS))
	captureUnavailable := false
	var encodedEpoch, encodedRecovery uint64
	captureStage := func(stageCtx context.Context) (capturedFrame, error) {
		if terminalActive.Load() {
			select {
			case <-peer.Done():
				return capturedFrame{}, io.EOF
			case <-stageCtx.Done():
				return capturedFrame{}, stageCtx.Err()
			case <-time.After(100 * time.Millisecond):
				return capturedFrame{}, streampipeline.Skip
			}
		}
		select {
		case <-stageCtx.Done():
			return capturedFrame{}, stageCtx.Err()
		case <-peer.Done():
			return capturedFrame{}, io.EOF
		case <-ticker.C:
		}
		if fps := int(captureFPS.Load()); fps > 0 && fps != captureTickFPS {
			ticker.Reset(time.Second / time.Duration(fps))
			captureTickFPS = fps
		}
		count := capturer.Count()
		if options.PrimaryDisplayOnly {
			count = min(count, 1)
		}
		displayMu.Lock()
		changed := false
		if selected < 0 || selected >= count {
			selected = 0
			changed = true
		}
		select {
		case request := <-nextDisplay:
			displayRequest = request.DisplayRequest
			lastDisplayReport = time.Time{}
			if request.Type == "display-select" {
				if request.Display != nil && *request.Display >= 0 && *request.Display < count && selected != *request.Display {
					selected = *request.Display
					changed = true
				}
			} else if count > 1 {
				selected = (selected + 1) % count
				changed = true
			}
		default:
		}
		bounds := agentView.view.Region.Bounds(capturer.Bounds(selected))
		if bounds != inputBounds || count != lastCount {
			changed = true
		}
		if changed {
			releaseInput()
			inputBounds = bounds
			captureEpoch.Add(1)
		}
		current := selected
		view := agentView.view
		epoch := captureEpoch.Load()
		displayMu.Unlock()
		if changed || time.Since(lastDisplayReport) >= time.Second {
			_ = peer.SendControl(p2p.Control{Type: "keyboard-capabilities", AppVersion: options.Version, EnhancementSupported: true, StreamCapabilities: streamconfig.Advertise(configSession)})
			_ = peer.SendControl(p2p.Control{Type: "displays", Display: &current, DisplayCount: count, DisplayRequest: displayRequest})
			lastDisplayReport = time.Now()
			lastCount = count
		}
		if count == 0 {
			if !captureUnavailable && options.OnState != nil {
				options.OnState("capture-unavailable")
			}
			captureUnavailable = true
			return capturedFrame{}, streampipeline.Skip
		}
		if view.Mode == "paused" {
			return capturedFrame{}, streampipeline.Skip
		}
		img, err := capturer.Capture(current)
		if fullFrameRequested.Swap(false) && errors.Is(err, desktop.ErrNoNewFrame) {
			// 靜止畫面也要嘗試提供完整影格，不能一直等下一次桌面變化。
			img, err = (desktop.ScreenshotCapturer{}).Capture(current)
		}
		if err != nil {
			if !errors.Is(err, desktop.ErrNoNewFrame) {
				slog.Warn("capture failed", "error", err)
				if !captureUnavailable && options.OnState != nil {
					options.OnState("capture-unavailable")
				}
				captureUnavailable = true
			}
			return capturedFrame{}, streampipeline.Skip
		}

		if captureUnavailable && options.OnState != nil {
			options.OnState("capture-restored")
		}
		captureUnavailable = false
		if view.Region != agentvideo.Full() {
			img = view.Region.Crop(img)
		}
		return capturedFrame{Image: img, Display: current, Epoch: epoch, ViewID: view.ViewID}, nil
	}
	encodeStage := func(stageCtx context.Context, raw capturedFrame) (encodedFrames, error) {
		if err := stageCtx.Err(); err != nil {
			return encodedFrames{}, err
		}
		displayMu.Lock()
		skip := agentView.view.Mode == "paused" || raw.ViewID != agentView.view.ViewID
		displayMu.Unlock()
		if skip {
			return encodedFrames{}, streampipeline.Skip
		}
		img, current := raw.Image, raw.Display
		var err error
		recoveryGeneration := recovery.Load()
		if encodedEpoch != raw.Epoch || encodedRecovery != recoveryGeneration {
			encodedEpoch, encodedRecovery = raw.Epoch, recoveryGeneration
			delta = desktop.DeltaEncoder{TileSize: desktop.DefaultTileSize, Quality: encodeQuality, Encoder: jpegEncoder, CompareBytes: policy.CompareBytes}
			if hardwareEncoder != nil {
				hardwareEncoder.Close()
				hardwareEncoder = nil
			}
			activeCodec = video.WireJPEG
			lastKeyframe = time.Time{}
		}
		profileChanged := false
		select {
		case request := <-profileRequests:
			// 視窗變大但仍受半解析度上限限制時，不必重建編碼器；
			// 實際影像尺寸變動由下方 encodedSize 統一處理。
			profileChanged = request.Profile != profile || request.Source != activeConfig.Source
			activeConfig = request
			enhancement = request.Source.Resolution.Mode != "inherit" && request.Source.Resolution.Mode != "native"
			profile = request.Profile
			encodeQuality = activeConfig.EncodeQuality(options.Quality)
		default:
		}
		if profileChanged {
			streamFPS = activeConfig.FrameRate(baseFPS)
			captureFPS.Store(int32(streamFPS))
			if hardwareEncoder != nil {
				hardwareEncoder.Close()
				hardwareEncoder = nil
			}
			activeCodec = video.WireJPEG
			lastKeyframe = time.Time{}
			delta.Quality = encodeQuality
		}
		sourceSize := img.Bounds().Size()
		dimensions := activeConfig.Dimensions(sourceSize)
		if dimensions != sourceSize {
			img = streamScaler.Scale(img, dimensions.X, dimensions.Y)
		} else {
			streamScaler.Reset()
		}
		size := img.Bounds().Size()
		enhancementReport = p2p.EnhancementReport{ConfigRevision: activeConfig.Revision, Enabled: enhancement, SourceWidth: sourceSize.X, SourceHeight: sourceSize.Y, StreamWidth: size.X, StreamHeight: size.Y}
		if size != encodedSize {
			encodedSize = size
			lastKeyframe = time.Time{}
			if hardwareEncoder != nil {
				hardwareEncoder.Close()
				hardwareEncoder = nil
			}
			activeCodec = video.WireJPEG
		}
		// 未收到新協定能力前維持 JPEG，避免舊版 遠端顯示 黑畫面。
		plan := codecPlan(policy, video.Codec(options.Codec), size.X, size.Y, remoteCodecs.Load(), remoteHardwareCodecs.Load(), failedCodecs, video.SupportsIntra, options.CodecGoal)
		desiredEncoder := plan[0]
		desired := video.WireForCodec(desiredEncoder)

		if desired != activeCodec || (desired != video.WireJPEG && desiredEncoder != activeEncoderCodec) {
			if hardwareEncoder != nil {
				hardwareEncoder.Close()
				hardwareEncoder = nil
			}
			activeCodec = desired
			if desired != video.WireJPEG {
				chosen := desiredEncoder
				activeEncoderCodec = chosen
				hardwareEncoder, err = video.NewIntraEncoder(chosen)
				if err != nil {
					failedCodecs[codecAttempt{activeEncoderCodec, (size.X + 1) &^ 1, (size.Y + 1) &^ 1}] = true
					activeCodec = video.WireJPEG
				}
				slog.Info("影像編碼協商", "codec", activeCodec)
			}
			lastKeyframe = time.Time{}
		}
		if hardwareEncoder != nil {
			if gopEncoder, ok := hardwareEncoder.(video.GOPEncoder); ok {
				gopEncoder.SetKeyframeInterval(configuredGOP())
			}
			if rateEncoder, ok := hardwareEncoder.(video.RateEncoder); ok {
				rate := activeConfig.Bitrate(sourceSize, baseFPS)
				rateEncoder.SetRate(rate, streamFPS)
				enhancementReport.Bitrate = rate
			}
			payload, encodeErr := hardwareEncoder.Encode(img, encodeQuality)
			keyframe := false
			if encodeErr == nil {
				keyframe, encodeErr = video.IsKeyframe(activeCodec, payload)
			}
			if encodeErr == nil {
				reportEncoding(activeCodec, activeEncoderCodec != video.CodecSoftwareAV1)
				if reporter, ok := hardwareEncoder.(video.BackendReporter); ok {
					backend := reporter.Backend()
					if backend != lastEncoderBackend {
						lastEncoderBackend = backend
						slog.Info("實際影片編碼後端", "backend", backend)
					}
				}
				seq++
				b := img.Bounds()
				return encodedFrames{ViewID: raw.ViewID, Frames: []p2p.Frame{{Display: current, Codec: byte(activeCodec), Sequence: seq, Width: uint32(b.Dx()), Height: uint32(b.Dy()), Keyframe: keyframe, JPEG: payload}}, Epoch: raw.Epoch, Recovery: recoveryGeneration, BitrateLimit: transmissionLimit()}, nil
			}
			slog.Warn("此編碼配置失敗，本張退回 JPEG，後續嘗試下一候選", "error", encodeErr)
			hardwareEncoder.Close()
			hardwareEncoder = nil
			failedCodecs[codecAttempt{activeEncoderCodec, (size.X + 1) &^ 1, (size.Y + 1) &^ 1}] = true
			activeCodec = video.WireJPEG
			lastKeyframe = time.Time{}
		}
		forceKeyframe := time.Since(lastKeyframe) >= 5*time.Second
		patches, err := delta.Encode(img, forceKeyframe)
		if err != nil {
			slog.Warn("delta encode failed", "error", err)
			lastKeyframe = time.Time{}
			return encodedFrames{}, streampipeline.Skip
		}
		reportEncoding(video.WireJPEG, jpegEncoder.Hardware())
		b := img.Bounds()
		batch := encodedFrames{ViewID: raw.ViewID, Epoch: raw.Epoch, Recovery: recoveryGeneration, BitrateLimit: transmissionLimit()}
		for _, patch := range patches {
			seq++
			batch.Frames = append(batch.Frames, p2p.Frame{Display: current, Sequence: seq, Width: uint32(b.Dx()), Height: uint32(b.Dy()), X: uint32(patch.X), Y: uint32(patch.Y), Keyframe: patch.Keyframe, JPEG: patch.JPEG})
			if patch.Keyframe {
				lastKeyframe = time.Now()
			}
		}
		if len(batch.Frames) == 0 {
			return encodedFrames{}, streampipeline.Skip
		}
		return batch, nil
	}
	sendStage := func(stageCtx context.Context, batch encodedFrames) error {
		agentView.gate.Lock()
		defer agentView.gate.Unlock()
		displayMu.Lock()
		paused := agentView.view.Mode == "paused"
		displayMu.Unlock()
		if paused {
			return nil
		}
		if batch.Epoch != captureEpoch.Load() || batch.Recovery != recovery.Load() {
			return nil
		}
		for _, frame := range batch.Frames {
			frame.ViewID = batch.ViewID
			if err := stageCtx.Err(); err != nil {
				return err
			}
			if err := peer.SendFrameLimited(stageCtx, frame, batch.BitrateLimit); err != nil {
				if errors.Is(err, p2p.ErrFrameDropped) {
					recovery.Add(1)
					return nil
				}
				return err
			}
		}
		return nil
	}
	err = streampipeline.Run(ctx, streampipeline.Stages[capturedFrame, encodedFrames]{Capture: captureStage, Encode: encodeStage, Send: sendStage})
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func handleControl(c input.Controller, e p2p.Control) {
	var err error
	switch e.Type {
	case "move":
		err = c.Move(e.X, e.Y)
	case "button":
		err = c.ButtonAt(e.Button, e.Down, e.X, e.Y)
	case "key":
		err = c.Key(e.Key, e.Down)
	case "wheel":
		err = c.Wheel(e.Delta)
	}
	if err != nil {
		slog.Warn("input event failed", "type", e.Type, "error", err)
	}
}
func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Disconnect 關閉本次已配對工作階段；接收連線的生命週期由呼叫者管理。
func Disconnect() {
	if peer := incomingPeer.Load(); peer != nil {
		_ = peer.Close()
	}
}
