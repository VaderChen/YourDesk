package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"log/slog"
	"math"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"yourdesk/internal/agentremote"
	"yourdesk/internal/branding"
	"yourdesk/internal/clipboard"
	"yourdesk/internal/p2p"
	"yourdesk/internal/rawkey"
	"yourdesk/internal/signaling"
	"yourdesk/internal/streamconfig"
	"yourdesk/internal/streampipeline"
	"yourdesk/internal/superres"
	"yourdesk/internal/video"
)

type game struct {
	agentHeadless             bool
	agentShow                 bool
	agentDeadline             time.Time
	agentButtons              map[int]p2p.Control
	agentRequests             <-chan agentremote.Request
	remoteVersion             atomic.Value
	windowFit                 windowFitState
	localShortcut             int
	enhancementIndicatorUntil time.Time
	renderedFrames            atomic.Uint64
	lastRenderFrame           *image.RGBA
	interpolationUsesML       bool
	lastInputMLRevision       uint64
	interpolation             frameInterpolator
	streamCapabilities        atomic.Pointer[streamconfig.Capabilities]
	streamResult              atomic.Pointer[streamconfig.Result]
	streamRequest             streamconfig.Request
	streamRevision            uint64
	ml                        *mlWorker
	superResolution           int32
	mlApplied                 bool
	remoteEnhancement         atomic.Pointer[p2p.EnhancementReport]
	lastEnhancementStatus     enhancementDisplayStatus

	disableKeyMapping               bool
	preferences                     *viewerPreferenceStore
	restorePreferences              bool
	restoreDisplay                  bool
	preferenceSample                time.Time
	presentedFrames                 atomic.Uint64 // 有新遠端影像的繪製次數，不計算重畫同一影格。
	rawSupported                    atomic.Bool
	rawActive                       bool
	rawHeld                         map[int]rawkey.Event
	quality                         int
	qualitySize                     [2]int
	qualityChangedAt, qualitySentAt time.Time
	qualitySent                     int

	uiEvents, firstPresented       bool
	clipboard                      *clipboard.Sync
	displayRequest                 uint64
	display, displayCount          int
	displayKnown, displayPending   bool
	frameDisplay, displayedDisplay int

	mode                         viewMode
	toolbarImage                 *ebiten.Image
	toolbarHover, toolbarCapture int
	toolbarDown                  bool

	finalTransform                  ebiten.GeoM
	finalWidth, finalHeight         int
	displayedWidth, displayedHeight int
	lastPixelMetrics                [6]int

	scaler                                      *imageScaler
	enhancer                                    *fsrScaler
	enhancementSupported                        atomic.Bool
	enhancementEnabled                          bool
	peer                                        *p2p.Peer
	mu                                          sync.RWMutex
	frame                                       *image.RGBA
	texture                                     *ebiten.Image
	dirty                                       bool
	width, height                               int
	lastButtons                                 map[ebiten.MouseButton]bool
	lastKeys                                    map[ebiten.Key]bool
	viewWidth, viewHeight                       int
	controlEnabled                              bool
	lastToggle                                  bool
	suppressFullscreenKey                       bool
	windowX, windowY, windowWidth, windowHeight int
}

func (g *game) isFullscreen() bool {
	if nativeTitlebarControls() {
		return nativeTitlebarOverlay()
	}
	return ebiten.IsFullscreen()
}

func (g *game) toggleFullscreen() {
	if nativeToggleFullscreen() {
		return
	}
	entering := !g.isFullscreen()
	if entering {
		g.windowWidth, g.windowHeight = ebiten.WindowSize()
		g.windowX, g.windowY = ebiten.WindowPosition()
	}
	ebiten.SetFullscreen(entering)
	// 使用引擎全螢幕的其他平台，在退出後還原視窗大小與位置。
	if !entering && !nativeRestoresWindowFrame() && g.windowWidth > 0 && g.windowHeight > 0 {
		ebiten.SetWindowSize(g.windowWidth, g.windowHeight)
		ebiten.SetWindowPosition(g.windowX, g.windowY)
	}
}

func (g *game) Update() error {
	// 遠端重啟後必須結束舊視窗與 signaling，讓管理介面釋放站台。
	// 與 MCP 隱藏模式使用相同的 P2P 生命週期，不依畫面是否更新判斷。
	if g.peer != nil {
		select {
		case <-g.peer.Done():
			slog.Info("遠端連線已結束，釋放遠端顯示工作階段")
			return ebiten.Termination
		default:
		}
	}
	g.updateAgent()
	g.updateWindowFit()
	if algorithm := viewerSuperResolution.Load(); algorithm != g.superResolution {
		g.superResolution = algorithm
		g.ml.reset()
	}
	enabled := viewerImageEnhancement.Load() && g.enhancementSupported.Load() && g.enhancer != nil
	if enabled != g.enhancementEnabled {
		g.enhancementEnabled = enabled
		g.ml.reset()
		slog.Info("畫面增強設定", "enabled", enabled, "backend", "FSR 1 EASU/RCAS", "interpolation", false)
		g.qualitySent = -1
		if g.enhancer != nil {
			g.enhancer.reset()
		}
	}
	if disabled := viewerDisableKeyMapping.Load(); disabled != g.disableKeyMapping {
		g.clipboard.CancelKeys("按鍵映射設定變更")
		clear(g.rawHeld)
		clear(g.lastKeys)
		g.disableKeyMapping = disabled
	}
	defer g.updatePreferences()
	toolbarHandled, toolbarErr := g.updateToolbar()
	if toolbarErr != nil {
		return toolbarErr
	}
	rawInput := g.updateRawKeys()
	if action := g.windowShortcut(rawInput); action != 0 {
		return g.executeWindowShortcut(action)
	}
	// 此組合鍵由本機處理，不把切換組合鍵傳到遠端。
	fullscreenKey := ebiten.IsKeyPressed(ebiten.KeyF)
	if !rawInput && ebiten.IsFocused() && fullscreenKey && ebiten.IsKeyPressed(ebiten.KeyShift) && (ebiten.IsKeyPressed(ebiten.KeyControl) || ebiten.IsKeyPressed(ebiten.KeyMeta)) {
		if !g.suppressFullscreenKey {
			g.toggleFullscreen()
		}
		g.suppressFullscreenKey = true
	}
	if g.suppressFullscreenKey {
		for key, down := range g.lastKeys {
			if down && g.peer != nil {
				_ = g.sendControl(p2p.Control{Type: "key", Key: commonKeys[key], Down: false})
			}
			g.lastKeys[key] = false
		}
		if fullscreenKey {
			return nil
		}
		g.suppressFullscreenKey = false
	}

	if nativeFullscreenRequested() {
		g.toggleFullscreen()
	}
	// 持續套用本機游標可見狀態，避免輸入文字或焦點切換後被系統隱藏。
	// 即使 F12 暫停遠端控制，本機游標仍須保持可見。
	if ebiten.IsFocused() {
		ebiten.SetCursorMode(ebiten.CursorModeVisible)
		showNativeCursor()
	}
	if g.peer == nil {
		return nil
	}
	// Only the focused Remote window may control the Host. This prevents a
	// same-machine smoke test from continuously fighting the local cursor.
	if !ebiten.IsFocused() || nativeTitlebarPopupOpen() {
		if time.Now().After(g.agentDeadline) {
			g.clipboard.CancelKeys("遠端顯示 失去焦點或開啟本機選單")
		}
		return nil
	}
	// F12 is a local safety switch: release control so the local pointer can
	// be recovered during same-machine testing.
	toggle := !rawInput && ebiten.IsKeyPressed(ebiten.KeyF12)
	if toggle && !g.lastToggle {
		g.controlEnabled = !g.controlEnabled
		for k := range g.lastButtons {
			g.lastButtons[k] = false
		}
		for k := range g.lastKeys {
			g.lastKeys[k] = false
		}
	}
	g.lastToggle = toggle
	if !g.controlEnabled || !g.displayInputReady() {
		g.clipboard.CancelKeys("遠端控制暫停或切換螢幕")
		return nil
	}
	iw, ih := g.displayedWidth, g.displayedHeight
	x, y := ebiten.CursorPosition()
	// 游標由引擎的邏輯座標還原至最終畫布，與實際影像共用像素區域。
	px, py := g.finalTransform.Apply(float64(x), float64(y))
	ox, oy, vw, vh := g.viewport(iw, ih)
	fx, fy := (px-ox)/vw, (py-oy)/vh
	inside := !toolbarHandled && py >= float64(g.toolbarHeight()) && iw > 0 && ih > 0 && fx >= 0 && fx <= 1 && fy >= 0 && fy <= 1
	fx, fy = math.Max(0, math.Min(1, fx)), math.Max(0, math.Min(1, fy))
	if inside {
		_ = g.sendControl(p2p.Control{Type: "move", X: fx, Y: fy})
	}
	for _, b := range []ebiten.MouseButton{ebiten.MouseButton0, ebiten.MouseButton1, ebiten.MouseButton2} {
		down := ebiten.IsMouseButtonPressed(b)
		if g.lastButtons[b] != down && (!down || inside) {
			n := 1 // YourDesk protocol: 1=left, 2=right, 3=middle
			if b == ebiten.MouseButton1 {
				n = 3
			}
			if b == ebiten.MouseButton2 {
				n = 2
			}
			_ = g.sendControl(p2p.Control{Type: "button", Button: n, Down: down, X: fx, Y: fy})
			g.lastButtons[b] = down
		}
	}
	if !rawInput {
		// 貼上屏障等待獨立剪貼簿通道完成，UI 不等待傳輸。
		if g.clipboard != nil && ebiten.IsKeyPressed(ebiten.KeyV) && !g.lastKeys[ebiten.KeyV] && (ebiten.IsKeyPressed(ebiten.KeyControl) || ebiten.IsKeyPressed(ebiten.KeyMeta)) {
			g.clipboard.Poll(true)
		}
		// 先按下修飾鍵、再送一般按鍵、最後放開修飾鍵，避免 map
		// 遍歷順序讓同一幀的 Ctrl／Cmd + V 變成單獨輸入 V。
		modifiers := []ebiten.Key{ebiten.KeyControl, ebiten.KeyMeta, ebiten.KeyShift, ebiten.KeyAlt}
		sendKey := func(key ebiten.Key) {
			down := ebiten.IsKeyPressed(key)
			if g.lastKeys[key] != down {
				_ = g.sendControl(p2p.Control{Type: "key", Key: commonKeys[key], Down: down})
				g.lastKeys[key] = down
			}
		}
		for _, key := range modifiers {
			if ebiten.IsKeyPressed(key) {
				sendKey(key)
			}
		}
		for key := range commonKeys {
			if key == ebiten.KeyControl || key == ebiten.KeyMeta || key == ebiten.KeyShift || key == ebiten.KeyAlt {
				continue
			}
			sendKey(key)
		}
		for _, key := range modifiers {
			if !ebiten.IsKeyPressed(key) {
				sendKey(key)
			}
		}

	}

	if _, delta := ebiten.Wheel(); delta != 0 && inside {
		_ = g.sendControl(p2p.Control{Type: "wheel", Delta: delta})
	}
	return nil
}

// 避免影像先畫到中間畫布，再被引擎縮放一次。
func (g *game) Draw(_ *ebiten.Image) {}

func (g *game) DrawFinalScreen(screen ebiten.FinalScreen, _ *ebiten.Image, geoM ebiten.GeoM) {
	g.finalTransform = geoM
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()
	if w < 1 || h < 1 {
		return
	}
	// 直接提交最終畫布，避免視窗動畫期間反覆配置全尺寸 GPU 中間影像。
	g.drawFrame(screen)
}

// 共用實際繪圖路徑，允許以獨立 GPU 畫布驗證像素，無須建立連線。
type viewerCanvas interface {
	Bounds() image.Rectangle
	Fill(color.Color)
	DrawImage(*ebiten.Image, *ebiten.DrawImageOptions)
}

func (g *game) drawFrame(screen viewerCanvas) {
	g.finalWidth, g.finalHeight = screen.Bounds().Dx(), screen.Bounds().Dy()
	defer g.drawToolbar(screen)
	applied := false
	g.mlApplied = false
	var renderWidth, renderHeight int
	defer func() { g.publishEnhancementStatus(applied, renderWidth, renderHeight) }()
	screen.Fill(color.Black)
	g.mu.Lock()
	dirty := g.dirty
	if g.frame != nil && g.enhancementEnabled && g.superResolution > 0 && (dirty || !g.ml.initialized) {
		g.ml.submit(g.frame, g.frameDisplay, int(g.superResolution)-1)
	}
	mlImage := g.ml.current()
	useML := g.enhancementEnabled && g.superResolution > 0 && mlImage != nil && g.ml.frame != nil && !nativeFullscreenTransitioning()
	baseFrame, baseDirty := g.frame, dirty
	if useML {
		baseFrame = g.ml.frame
		baseDirty = g.ml.revision != g.lastInputMLRevision
	}
	if useML != g.interpolationUsesML {
		g.interpolation.reset()
		baseDirty = true
	}
	g.interpolationUsesML = useML
	g.lastInputMLRevision = g.ml.revision
	iw, ih := 0, 0
	if g.frame != nil {
		iw, ih = g.frame.Bounds().Dx(), g.frame.Bounds().Dy()
	}
	// Core ML 只放大真實影格，再交給所選補幀方式；生成幀不再被另一個非同步佇列覆蓋。
	renderFrame, visualDirty := g.interpolation.frame(baseFrame, g.frameDisplay, baseDirty, viewerInterpolation.Load())
	if visualDirty && renderFrame != nil {
		if g.texture == nil || g.texture.Bounds() != renderFrame.Bounds() {
			if g.texture != nil {
				g.texture.Dispose()
			}
			g.texture = ebiten.NewImage(renderFrame.Bounds().Dx(), renderFrame.Bounds().Dy())
		}
		g.texture.WritePixels(renderFrame.Pix)
		g.displayedDisplay = g.frameDisplay
	}
	g.dirty = false
	img := g.texture
	g.mu.Unlock()
	if img == nil || iw == 0 || ih == 0 {
		return
	}
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterNearest}
	g.displayedWidth, g.displayedHeight = iw, ih
	ox, oy, vw, vh := g.viewport(iw, ih)
	renderWidth, renderHeight = int(vw), int(vh)
	metrics := [6]int{iw, ih, g.finalWidth, g.finalHeight, int(vw), int(vh)}
	if metrics != g.lastPixelMetrics {
		g.lastPixelMetrics = metrics
		slog.Info("遠端顯示 像素對應", "來源", fmt.Sprintf("%dx%d", iw, ih),
			"最終畫布", fmt.Sprintf("%dx%d", g.finalWidth, g.finalHeight),
			"顯示", fmt.Sprintf("%dx%d", int(vw), int(vh)), "點對點", int(vw) == iw && int(vh) == ih)
	}
	if useML {
		g.mlApplied = true
		applied = true
		op.Filter = ebiten.FilterLinear
		op.GeoM.Scale(vw/float64(img.Bounds().Dx()), vh/float64(img.Bounds().Dy()))
	} else if g.enhancementEnabled && int(vw) >= iw && int(vh) >= ih && !nativeFullscreenTransitioning() {
		img = g.enhancer.Scale(img, int(vw), int(vh), visualDirty)
		applied = img == g.enhancer.output
		op.Filter = ebiten.FilterLinear
		op.GeoM.Scale(vw/float64(img.Bounds().Dx()), vh/float64(img.Bounds().Dy()))
	} else if g.scaler != nil && g.scaler.ready(img, int(vw), int(vh), visualDirty) {
		img = g.scaler.Scale(img, int(vw), int(vh), visualDirty)
	} else {
		// 尺寸變動期間與放大模式直接取樣原圖，維持畫面更新。
		op.Filter = ebiten.FilterLinear
		op.DisableMipmaps = true
		op.GeoM.Scale(vw/float64(img.Bounds().Dx()), vh/float64(img.Bounds().Dy()))
	}
	op.GeoM.Translate(ox, oy)
	screen.DrawImage(img, op)
	newPresentation := visualDirty
	if viewerInterpolation.Load() {
		newPresentation = renderFrame != g.lastRenderFrame
	}
	if newPresentation {
		g.renderedFrames.Add(1)
	}
	g.lastRenderFrame = renderFrame
	if dirty {
		g.presentedFrames.Add(1)
	}
	if g.uiEvents && !g.firstPresented {
		g.firstPresented = true
		emitUIEvent("frame", "")
	}
}

// 繪圖與輸入共用同一個等比例區域，黑邊不傳送遠端點擊。
func imageViewport(w, h, iw, ih int) (float64, float64, float64, float64) {
	// 最多以來源像素 1:1 顯示；較大的視窗保留黑邊，不放大影像。
	scale := math.Min(1, math.Min(float64(max(1, w))/float64(max(1, iw)), float64(max(1, h))/float64(max(1, ih))))
	vw, vh := math.Max(1, math.Floor(float64(max(1, iw))*scale)), math.Max(1, math.Floor(float64(max(1, ih))*scale))
	return math.Floor((float64(w) - vw) / 2), math.Floor((float64(h) - vh) / 2), vw, vh
}
func (g *game) Layout(outW, outH int) (int, int) {
	w, h := g.LayoutF(float64(outW), float64(outH))
	return int(w), int(h)
}

func main() {
	defer nativeCloseTitlebar()
	signalURL := flag.String("signal", "wss://127.0.0.1:8080/ws", "rendezvous WebSocket URL")
	name := flag.String("name", "", "遠端顯示 顯示名稱")
	room := flag.String("room", "", "pairing room")
	secretStdin := flag.Bool("secret-stdin", false, "從標準輸入的私有管道讀取連線密碼")
	codec := flag.String("codec", "auto", "decoder mode: auto, hardware, software")
	interactiveAuth := flag.Bool("interactive-auth", false, "透過 Client UI 輸入配對密碼")
	flag.IntVar(&sourceFPSLimit, "source-fps", 20, "來源串流 FPS 上限（5～60）")
	flag.Bool("mcp-managed", false, "由 MCP 管理的遠端連線")
	mcpHidden := flag.Bool("mcp-hidden", false, "MCP 背景操作，直到使用者開啟遠端畫面")
	backgroundDiagnostic := flag.Bool("diagnostic", false, "背景串流診斷，不開啟 遠端顯示")
	flag.Parse()
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "source-fps" {
			sourceFPSOverride = true
		}
	})
	if sourceFPSLimit < 5 || sourceFPSLimit > 60 {
		slog.Error("來源 FPS 上限必須為 5～60")
		os.Exit(2)
	}
	if *room == "" {
		fatal(fmt.Errorf("必須提供 -room"))
	}
	if !*secretStdin {
		fatal(fmt.Errorf("必須透過標準輸入提供連線密碼"))
	}
	if *backgroundDiagnostic {
		if err := runBackgroundDiagnostic(*signalURL, *room, *codec); err != nil {
			fatal(err)
		}
		return
	}
	passwords := newPasswordInput()
	secret, err := passwords.read(context.Background(), false)
	if err != nil {
		fatal(err)
	}
	jpegDecoder, decoderSelection := video.NewJPEGDecoderForCodec(*codec)
	defer jpegDecoder.Close()
	slog.Info("video decoder selected", "hardware", decoderSelection.Hardware, "backend", decoderSelection.Backend, "codec", decoderSelection.Codec, "probed", decoderSelection.Probed, "detail", decoderSelection.Detail)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var sig *signaling.Client
	if address, direct := signaling.DirectAddress(*room); direct {
		sig, err = signaling.DialDirect(ctx, address, secret)
	} else {
		sig, err = signaling.Dial(ctx, *signalURL, *room, signaling.RoleViewer, secret)
	}
	if err != nil {
		fatal(err)
	}
	defer sig.Close()
	if *interactiveAuth {
		sig.ResolveSecret = passwords.resolver()
	}
	var videoDecodeFailed atomic.Bool
	var videoDecoder video.DecodeSession
	defer videoDecoder.Close()
	lastDecoderBackend := ""
	var codecStatusMu sync.Mutex
	sourceModes := map[byte]string{}
	receivedCodec := ""
	var receivedWire byte
	receivedMode := "unknown"

	scaler, err := newImageScaler()
	if err != nil {
		fatal(fmt.Errorf("縮圖著色器初始化失敗：%w", err))
	}
	defer scaler.Close()
	superres.StartProbe()
	ml := newMLWorker()
	defer ml.Close()
	enhancer, enhanceErr := newFSRScaler()
	if enhanceErr != nil {
		slog.Warn("FSR 1 shader 無法編譯，保留原始串流", "error", enhanceErr)
	}
	if enhancer != nil {
		defer enhancer.Close()
	}
	g := &game{agentHeadless: *mcpHidden, agentRequests: passwords.agent, rawHeld: make(map[int]rawkey.Event), quality: 1, qualitySent: -1, uiEvents: *interactiveAuth, clipboard: clipboard.New(), scaler: scaler, ml: ml, enhancer: enhancer, lastButtons: make(map[ebiten.MouseButton]bool), lastKeys: make(map[ebiten.Key]bool), controlEnabled: true}
	defer g.interpolation.reset()
	handshakeCtx, cancelHandshake := context.WithTimeout(ctx, 90*time.Second)
	defer cancelHandshake()
	g.preferences = openViewerPreferences(*signalURL, *room)
	language := refreshViewerLanguage("")
	defer g.preferences.Close()
	g.mode = viewMode(g.preferences.current.Scale)
	g.quality = g.preferences.current.Quality
	g.controlEnabled = g.preferences.current.Control
	g.restorePreferences, g.restoreDisplay = true, true
	var stats viewerPipelineStats
	var lastVideoSequence uint64
	var lastVideoDisplay int
	decodeFrames := streampipeline.NewConsumer(ctx, func(f p2p.Frame) {
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
		receivedCodec = map[byte]string{0: "JPEG", 1: "H.264", 2: "HEVC"}[f.Codec]
		receivedMode = mode
		codecStatusMu.Unlock()
		g.mu.Lock()
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
		g.frameDisplay = f.Display
		if g.frame == nil || g.frame.Bounds().Dx() != int(f.Width) || g.frame.Bounds().Dy() != int(f.Height) || f.Keyframe {
			g.frame = image.NewRGBA(image.Rect(0, 0, int(f.Width), int(f.Height)))
		}
		dst := image.Rect(int(f.X), int(f.Y), int(f.X)+img.Bounds().Dx(), int(f.Y)+img.Bounds().Dy())
		draw.Draw(g.frame, dst, img, img.Bounds().Min, draw.Src)
		g.dirty = true
		g.width, g.height = int(f.Width), int(f.Height)
		g.mu.Unlock()
	})
	defer decodeFrames.Close()
	peer, err := p2p.NewViewer(handshakeCtx, sig, func(f p2p.Frame) {
		stats.received.Add(1)
		stats.wire.Store(uint32(f.Codec))
		// 封包已重組為獨立記憶體，交給解碼 worker 後即可接收下一幀。
		decodeFrames.Submit(f)
	}, g.receiveDisplays, func(c p2p.Control) {
		if g.clipboard.Handle(c) {
			return
		}
		if c.Type == "video-status" {
			g.remoteEnhancement.Store(c.EnhancementReport)
			if c.VideoCodec <= 2 && (c.EncodingMode == "hardware" || c.EncodingMode == "software") {
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
		fatal(err)
	}
	// 定期公告可接收格式，避免初次 DataChannel 開啟時序遺失協商。
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		var codecs []byte
		if *codec != "software" && *codec != "software-jpeg" {
			for _, cap := range video.IntraCapabilities() {
				if cap.Decode {
					codecs = append(codecs, byte(video.WireForCodec(cap.Codec)))
				}
			}
		}
		// 背景每秒取樣，避免統計收集阻塞繪圖；以實際間隔計算速度。
		lastSent, lastReceived := peer.TrafficBytes()
		lastSample := time.Now()
		diagnostic := newPipelineSample(&stats, g)
		_, diagnostic.bytes = peer.TrafficBytes()
		lastFrames := g.presentedFrames.Load()
		lastRendered := g.renderedFrames.Load()
		for {
			language = refreshViewerLanguage(language)
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
			advertised := codecs
			if videoDecodeFailed.Load() {
				advertised = nil
			}
			_ = peer.SendControl(p2p.Control{Type: "video-capabilities", Codecs: advertised, KeyframeInterval: 10})
			select {
			case <-ctx.Done():
				return
			case <-peer.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	emitUIEvent("authenticated", "")
	g.peer = peer
	go g.clipboard.Run(ctx, peer)
	defer func() {
		// 先解除收件回呼的反壓並等待解碼，再關閉網路及原生資源。
		decodeFrames.Close()
		peer.Close()
	}()
	if *mcpHidden && !g.runAgentHidden(ctx) {
		return
	}
	displayName := strings.TrimSpace(*name)
	if displayName == "" {
		displayName = "YourDesk"
	}
	ebiten.SetWindowTitle(displayName + " / " + *room)
	if icon := branding.Icon(); icon != nil {
		ebiten.SetWindowIcon([]image.Image{icon})
	}
	ebiten.SetCursorMode(ebiten.CursorModeVisible)
	// Agent 由私有管道操作時，失焦仍須處理更新與回應。
	ebiten.SetRunnableOnUnfocused(true)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowSizeLimits(640, 270, -1, -1)
	ebiten.SetWindowSize(g.preferences.current.Width, g.preferences.current.Height)
	nativePrepareViewerWindow()
	if err := ebiten.RunGame(g); err != nil {
		fatal(err)
	}
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func fatal(err error) {
	message := "遠端連線失敗，請確認遠端 Client 與網路狀態後再試。"
	if strings.Contains(err.Error(), "時間超出") {
		message = "遠端握手資料已過期，請重新啟動遠端 Client，並確認雙方系統時間正確。"
	} else if strings.Contains(err.Error(), "deadline exceeded") {
		message = "等待遠端握手逾時；Client 已註冊，但尚未完成連線。請更新或重新啟動遠端 Client 後再試。"
	} else if strings.Contains(err.Error(), "signaling") || strings.Contains(err.Error(), "IP 直連") {
		message = "無法完成連線服務握手，請檢查網路與遠端 Client 狀態。"
	}
	emitUIEvent("error", message)
	fmt.Fprintln(os.Stderr, "yourdesk-remote:", err)
	os.Exit(1)
}

var commonKeys = map[ebiten.Key]string{
	ebiten.KeyA: "a", ebiten.KeyB: "b", ebiten.KeyC: "c", ebiten.KeyD: "d", ebiten.KeyE: "e",
	ebiten.KeyF: "f", ebiten.KeyG: "g", ebiten.KeyH: "h", ebiten.KeyI: "i", ebiten.KeyJ: "j",
	ebiten.KeyK: "k", ebiten.KeyL: "l", ebiten.KeyM: "m", ebiten.KeyN: "n", ebiten.KeyO: "o",
	ebiten.KeyP: "p", ebiten.KeyQ: "q", ebiten.KeyR: "r", ebiten.KeyS: "s", ebiten.KeyT: "t",
	ebiten.KeyU: "u", ebiten.KeyV: "v", ebiten.KeyW: "w", ebiten.KeyX: "x", ebiten.KeyY: "y",
	ebiten.KeyZ: "z", ebiten.KeyEnter: "enter", ebiten.KeyEscape: "escape", ebiten.KeySpace: "space",
	ebiten.KeyBackspace: "backspace", ebiten.KeyTab: "tab",
	ebiten.KeyDigit0: "0", ebiten.KeyDigit1: "1", ebiten.KeyDigit2: "2", ebiten.KeyDigit3: "3", ebiten.KeyDigit4: "4",
	ebiten.KeyDigit5: "5", ebiten.KeyDigit6: "6", ebiten.KeyDigit7: "7", ebiten.KeyDigit8: "8", ebiten.KeyDigit9: "9",
	ebiten.KeyArrowUp: "arrowup", ebiten.KeyArrowDown: "arrowdown", ebiten.KeyArrowLeft: "arrowleft", ebiten.KeyArrowRight: "arrowright",
	ebiten.KeyDelete: "delete", ebiten.KeyHome: "home", ebiten.KeyEnd: "end", ebiten.KeyPageUp: "pageup", ebiten.KeyPageDown: "pagedown",
	ebiten.KeyShift: "shift", ebiten.KeyControl: "control", ebiten.KeyAlt: "alt", ebiten.KeyMeta: "meta",
	ebiten.KeyF1: "f1", ebiten.KeyF2: "f2", ebiten.KeyF3: "f3", ebiten.KeyF4: "f4", ebiten.KeyF5: "f5", ebiten.KeyF6: "f6",
	ebiten.KeyF7: "f7", ebiten.KeyF8: "f8", ebiten.KeyF9: "f9", ebiten.KeyF10: "f10", ebiten.KeyF11: "f11", ebiten.KeyF12: "f12",
}

// 直接以目前螢幕的實體像素繪圖，避免 Retina 先縮小再放大。
// Ebitengine 同時將游標座標轉換至此座標空間，維持點擊與黑邊一致。
func (g *game) LayoutF(outW, outH float64) (float64, float64) {
	scale := ebiten.Monitor().DeviceScaleFactor()
	g.viewWidth = max(1, int(math.Ceil(outW*scale)))
	g.viewHeight = max(1, int(math.Ceil(outH*scale)))
	return float64(g.viewWidth), float64(g.viewHeight)
}
