package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"io"
	"log/slog"
	"math"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"yourdesk/internal/authlog"
	"yourdesk/internal/clientui"
	"yourdesk/internal/clipboard"
	"yourdesk/internal/desktop"
	"yourdesk/internal/deviceid"
	"yourdesk/internal/hostguard"
	"yourdesk/internal/input"
	"yourdesk/internal/p2p"
	"yourdesk/internal/prelogin"
	"yourdesk/internal/rawkey"
	"yourdesk/internal/security"
	"yourdesk/internal/signaling"
	"yourdesk/internal/streamconfig"
	"yourdesk/internal/streampipeline"
	"yourdesk/internal/video"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--prelogin" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		_, err := prelogin.Handle(ctx, os.Args[1:])
		if err != nil {
			fmt.Fprintln(os.Stderr, "YourDesk 未登入服務：", err)
			os.Exit(1)
		}
		return
	}

	signalURL := flag.String("signal", "wss://127.0.0.1:8080/ws", "rendezvous WebSocket URL")
	room := flag.String("room", "", "pairing room; empty means use hardware UID")
	serviceHost := flag.Bool("prelogin-host", false, "由系統服務授權的 Host")
	parentStdin := flag.Bool("parent-stdin", false, "管理 APP 關閉密碼管線時一併結束 Host")
	secretStdin := flag.Bool("secret-stdin", false, "從標準輸入讀取連線密碼 JSON；供自動化使用")
	display := flag.Int("display", 0, "display index")
	fps := flag.Int("fps", 12, "maximum capture FPS")
	quality := flag.Int("quality", 70, "JPEG quality 1-100")
	printSecret := flag.Bool("print-secret", false, "讀取或首次建立本機連線密碼後輸出並結束")
	printUID := flag.Bool("print-uid", false, "print hardware-derived device UID and exit")
	codec := flag.String("codec", "auto", "codec mode: auto, hardware-h264, hardware-hevc, software-jpeg")
	ui := flag.Bool("ui", false, "開啟內嵌 HTML 的 Client 桌面視窗")
	directListen := flag.String("direct-listen", "", "IP 直連 TLS 監聽位址；必須明確啟用")
	checkSignal := flag.Bool("check-signal", false, "驗證 TLS 伺服器健康狀態後結束")
	flag.Parse()
	mode := "host"
	if *ui {
		mode = "ui"
	}
	authlog.Start(clientui.ApplicationVersion(), mode)
	if *checkSignal {
		if err := security.CheckSignalServer(*signalURL); err != nil {
			fatal(err)
		}
		fmt.Println("TLS 伺服器驗證成功")
		return
	}

	if *printSecret {
		value, _, err := security.LocalSecret("")
		if err != nil {
			fatal(err)
		}
		fmt.Println(value)
		return
	}
	if *ui {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		err := clientui.Run(ctx, clientui.Options{Signal: *signalURL, Room: *room, DirectListen: *directListen,
			HostArgs: []string{"-signal", *signalURL, "-room", *room,
				"-display", fmt.Sprint(*display), "-fps", fmt.Sprint(*fps), "-quality", fmt.Sprint(*quality), "-codec", *codec}})
		if err != nil {
			fatal(err)
		}
		return
	}

	if *printUID {
		identity, err := deviceid.Current()
		if err != nil {
			fatal(err)
		}
		fmt.Println(identity.UID)
		return
	}
	if *room == "" {
		identity, err := deviceid.Current()
		if err != nil {
			fatal(err)
		}
		*room = identity.UID
		fmt.Printf("DEVICE UID: %s (%s)\n", identity.UID, identity.Source)
	}
	secretText := ""
	challenge := ""
	var parentScanner *bufio.Scanner
	if *parentStdin && !*secretStdin {
		fatal(errors.New("parent-stdin 必須搭配 secret-stdin"))
	}
	if *secretStdin {
		var input struct {
			Secret    string `json:"secret"`
			Challenge string `json:"challenge"`
		}
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Buffer(make([]byte, 1024), 4096)
		if !scanner.Scan() || json.Unmarshal(scanner.Bytes(), &input) != nil {
			fatal(fmt.Errorf("無法從標準輸入讀取連線密碼"))
		}
		parentScanner = scanner
		secretText = input.Secret
		challenge = input.Challenge
	} else {
		var err error
		secretText, _, err = security.LocalSecret("")
		if err != nil {
			fatal(err)
		}
	}
	authlog.Event("password-loaded", map[string]any{"fromPrivatePipe": *secretStdin})
	secret, err := security.DecodeSecret(secretText)
	if err != nil {
		fatal(err)
	}
	authlog.Event("password-decoded", map[string]any{"valid": err == nil})
	if challenge != "" {
		proof := security.Sign(secret, []byte(challenge))
		data, _ := json.Marshal(map[string]string{"event": "host-password-proof", "proof": proof})
		fmt.Println("YOURDESK_UI_EVENT " + string(data))
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *parentStdin {
		// 此管線由 UI 持有；正常退出或崩潰都會關閉，不輪詢可重用的 PID。
		go func() {
			for parentScanner.Scan() {
				var request struct {
					Disconnect bool `json:"disconnect"`
				}
				if json.Unmarshal(parentScanner.Bytes(), &request) == nil && request.Disconnect {
					if peer := incomingPeer.Load(); peer != nil {
						_ = peer.Close()
					}
				}
			}
			authlog.Event("parent-pipe-closed", nil)
			stop()
			// 擷取驅動若無法配合取消，也不允許 Host 永久殘留。
			time.Sleep(5 * time.Second)
			os.Exit(0)
		}()
	}
	if *serviceHost {
		if !*parentStdin || !*secretStdin {
			fatal(errors.New("服務 Host 必須由私有父管線啟動"))
		}
		release, err := prelogin.AcquireHost(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			fatal(err)
		}
		defer release()
	}
	releaseHost, err := hostguard.Acquire(ctx, *room, func() {
		fmt.Println(`YOURDESK_UI_EVENT {"event":"host-conflict","local":true}`)
		authlog.Event("local-host-occupied", nil)
	})
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		fatal(err)
	}
	defer releaseHost()
	fmt.Println(`YOURDESK_UI_EVENT {"event":"host-ready"}`)
	options := hostOptions{Display: *display, FPS: *fps, Quality: *quality, Codec: *codec}
	if *directListen != "" {
		go func() {
			err := signaling.ListenDirect(ctx, *directListen, secret, func(session context.Context, sig *signaling.Client) {
				if err := streamHost(session, sig, options); err != nil {
					slog.Warn("IP 直連結束", "error", err)
				}
			})
			if err != nil {
				slog.Error("IP 直連無法啟動", "error", err)
			}
		}()
	}
	// 中央服務失聯時持續重試；不影響獨立的 IP 直連入口。
	for ctx.Err() == nil {
		sig, err := signaling.Dial(ctx, *signalURL, *room, signaling.RoleHost, secret)
		if err == nil {
			err = streamHost(ctx, sig, options)
			sig.Close()
		}
		if err != nil {
			if errors.Is(err, signaling.ErrHostOccupied) {
				fmt.Println(`YOURDESK_UI_EVENT {"event":"host-conflict"}`)
			}
			authlog.Event("host-session-ended", map[string]any{"class": authlog.ErrorClass(err)})
			slog.Warn("中央連線結束，稍後重試", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

var incomingGeneration atomic.Uint64
var incomingPeer atomic.Pointer[p2p.Peer]
var activeSession atomic.Bool

type hostOptions struct {
	Display, FPS, Quality int
	Codec                 string
}

func streamHost(ctx context.Context, sig *signaling.Client, options hostOptions) error {
	switch video.Codec(options.Codec) {
	case video.CodecAuto, video.CodecHardwareH264, video.CodecHardwareHEVC, video.CodecSoftwareH264, video.CodecHardwareJPEG, video.CodecSoftwareJPEG:
	default:
		return fmt.Errorf("不支援的影像編碼：%s", options.Codec)
	}
	// JPEG 保留作為舊 遠端顯示 與硬體失敗時的相容路徑。
	jpegEncoder, jpegSelection := video.NewJPEGEncoderForCodec(video.Codec(options.Codec))
	defer jpegEncoder.Close()
	slog.Info("JPEG 相容編碼器", "selected", jpegSelection.Selected)
	var remoteCodecs atomic.Uint32
	var hardwareEncoder video.IntraEncoder
	var activeCodec video.WireCodec
	var hardwareFailed bool
	defer func() {
		if hardwareEncoder != nil {
			hardwareEncoder.Close()
		}
	}()
	capturer := desktop.NewLiveCapturer()
	defer capturer.Close()
	selected := options.Display
	var displayMu sync.Mutex
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
	clipboardSync := clipboard.New()
	var authorized atomic.Bool
	peer, err := p2p.NewHost(ctx, sig, func(c p2p.Control) {
		if c.Type == "video-capabilities" {
			if c.KeyframeInterval == 10 {
				remoteGOP.Store(10)
			} else {
				remoteGOP.Store(1)
			}
			var mask uint32
			for _, codec := range c.Codecs {
				if codec == byte(video.WireH264) || codec == byte(video.WireHEVC) {
					mask |= 1 << codec
				}
			}
			remoteCodecs.Store(mask)
			return
		}
		if !authorized.Load() {
			return
		}
		if clipboardSync.Handle(c) {
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
					fmt.Printf("YOURDESK_UI_EVENT {\"event\":\"host-connected\",\"session\":%d}\n", sessionGeneration)
					select {
					case <-ctx.Done():
					case <-peer.Done():
					}
					fmt.Printf("YOURDESK_UI_EVENT {\"event\":\"host-disconnected\",\"session\":%d}\n", sessionGeneration)
					return
				}
			}
		}
	}()
	configPeer.Store(peer)
	authorized.Store(true)
	go clipboardSync.Run(ctx, peer)
	fmt.Println(`YOURDESK_UI_EVENT {"event":"host-ready"}`)
	fmt.Println(`YOURDESK_UI_EVENT {"event":"authenticated"}`)
	fmt.Println("P2P direct UDP session established; press Ctrl-C to stop")

	baseFPS := max(1, options.FPS)
	streamFPS := baseFPS
	captureTickFPS := baseFPS
	interval := time.Second / time.Duration(streamFPS)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var seq uint64
	delta := desktop.DeltaEncoder{TileSize: desktop.DefaultTileSize, Quality: clamp(options.Quality, 1, 100), Encoder: jpegEncoder}
	lastKeyframe := time.Now().Add(-time.Hour)
	var lastDisplayReport time.Time
	lastCount := -1
	profile := "standard"
	enhancement := false
	activeConfig := streamconfig.Compose("standard", 8192, 8192, false)
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
			if activeConfig.Source.Rate.Mode == "cbr" && result.Effective.Bitrate == 0 {
				result.Error = "編碼後端不支援目前碼率要求，已回退品質控制"
			}
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
	var captureEpoch, recovery atomic.Uint64
	var encodedEpoch, encodedRecovery uint64
	captureStage := func(stageCtx context.Context) (capturedFrame, error) {
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
		bounds := capturer.Bounds(selected)
		if bounds != inputBounds || count != lastCount {
			changed = true
		}
		if changed {
			releaseInput()
			inputBounds = bounds
			captureEpoch.Add(1)
		}
		current := selected
		displayMu.Unlock()
		if changed || time.Since(lastDisplayReport) >= time.Second {
			_ = peer.SendControl(p2p.Control{Type: "keyboard-capabilities", AppVersion: clientui.ApplicationVersion(), EnhancementSupported: true, StreamCapabilities: streamconfig.Advertise(configSession)})
			_ = peer.SendControl(p2p.Control{Type: "displays", Display: &current, DisplayCount: count, DisplayRequest: displayRequest})
			lastDisplayReport = time.Now()
			lastCount = count
		}
		if count == 0 {
			return capturedFrame{}, streampipeline.Skip
		}
		img, err := capturer.Capture(current)
		if err != nil {
			if !errors.Is(err, desktop.ErrNoNewFrame) {
				slog.Warn("capture failed", "error", err)
			}
			return capturedFrame{}, streampipeline.Skip
		}

		return capturedFrame{Image: img, Display: current, Epoch: captureEpoch.Load()}, nil
	}
	encodeStage := func(stageCtx context.Context, raw capturedFrame) (encodedFrames, error) {
		if err := stageCtx.Err(); err != nil {
			return encodedFrames{}, err
		}
		img, current := raw.Image, raw.Display
		var err error
		recoveryGeneration := recovery.Load()
		if encodedEpoch != raw.Epoch || encodedRecovery != recoveryGeneration {
			encodedEpoch, encodedRecovery = raw.Epoch, recoveryGeneration
			delta = desktop.DeltaEncoder{TileSize: desktop.DefaultTileSize, Quality: encodeQuality, Encoder: jpegEncoder}
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
		desired := video.WireJPEG
		if !hardwareFailed {
			for _, candidate := range []video.Codec{video.CodecHardwareHEVC, video.CodecHardwareH264} {
				wire := video.WireForCodec(candidate)
				requested := video.Codec(options.Codec)
				if (requested == video.CodecAuto || requested == candidate) && remoteCodecs.Load()&(1<<wire) != 0 && video.SupportsIntra(candidate) {
					desired = wire
					break
				}
			}
		}
		if desired != activeCodec {
			if hardwareEncoder != nil {
				hardwareEncoder.Close()
				hardwareEncoder = nil
			}
			activeCodec = desired
			if desired != video.WireJPEG {
				chosen := video.CodecHardwareH264
				if desired == video.WireHEVC {
					chosen = video.CodecHardwareHEVC
				}
				hardwareEncoder, err = video.NewIntraEncoder(chosen)
				if err != nil {
					hardwareFailed = true
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
				reportEncoding(activeCodec, true)
				if reporter, ok := hardwareEncoder.(video.BackendReporter); ok {
					backend := reporter.Backend()
					if backend != lastEncoderBackend {
						lastEncoderBackend = backend
						slog.Info("實際影片編碼後端", "backend", backend)
					}
				}
				seq++
				b := img.Bounds()
				return encodedFrames{Frames: []p2p.Frame{{Display: current, Codec: byte(activeCodec), Sequence: seq, Width: uint32(b.Dx()), Height: uint32(b.Dy()), Keyframe: keyframe, JPEG: payload}}, Epoch: raw.Epoch, Recovery: recoveryGeneration}, nil
			}
			slog.Warn("硬體壓縮失敗，退回 JPEG", "error", encodeErr)
			hardwareEncoder.Close()
			hardwareEncoder = nil
			hardwareFailed = true
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
		batch := encodedFrames{Epoch: raw.Epoch, Recovery: recoveryGeneration}
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
		if batch.Epoch != captureEpoch.Load() || batch.Recovery != recovery.Load() {
			return nil
		}
		for _, frame := range batch.Frames {
			if err := stageCtx.Err(); err != nil {
				return err
			}
			if err := peer.SendFrameChecked(frame); err != nil {
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
func fatal(err error) { fmt.Fprintln(os.Stderr, "yourdesk-client:", err); os.Exit(1) }
