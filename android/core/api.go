package core

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
	"unicode/utf8"
	"yourdeskandroid/core/internal/p2p"
	"yourdeskandroid/core/internal/security"
	"yourdeskandroid/core/internal/signaling"
	"yourdeskandroid/core/internal/streamconfig"
)

type FrameInfo struct {
	Sequence      uint64
	Display       int
	Width, Height int
	Codec         int
	Keyframe      bool
	Bytes         int
}
type Viewer struct {
	mu         sync.Mutex
	generation uint64
	peer       *p2p.Peer
	cancel     context.CancelFunc
	onFrame    func(FrameInfo)
	onState    func(string)
	frameMu    sync.Mutex
	// JPEG 模式會把一張桌面拆成多個區塊影格；不能只保留最後一筆，
	// 否則 Android 尚未讀取前面的區塊時，合成基底就會缺塊。
	frames             []*p2p.Frame
	frameGeneration    uint64
	frameBytes         int
	awaitingKeyframe   bool
	recoveryPending    bool
	streamSendMu       sync.Mutex
	streamCapabilities *streamconfig.Capabilities
	streamRevision     uint64
	streamRequest      *streamconfig.Request
	streamResult       *streamconfig.Result
	streamError        string
}

func NewViewer() *Viewer                            { return &Viewer{awaitingKeyframe: true} }
func (v *Viewer) SetFrameHandler(h func(FrameInfo)) { v.mu.Lock(); v.onFrame = h; v.mu.Unlock() }
func (v *Viewer) SetStateHandler(h func(string))    { v.mu.Lock(); v.onState = h; v.mu.Unlock() }
func (v *Viewer) state(s string) {
	v.mu.Lock()
	h := v.onState
	v.mu.Unlock()
	if h != nil {
		h(s)
	}
}

// Connect 等待認證、ICE 與命令列能力完成才回傳。
func (v *Viewer) Connect(url, room, secret string) error {
	// Register the new cancellation before notifying observers or closing the old
	// peer: Close must also cancel a connection still preparing its first dial.
	ctx, cancel, generation := v.beginConnect()
	raw, err := security.DecodeSecret(secret)
	if err != nil {
		cancel()
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	timer := time.AfterFunc(90*time.Second, cancel)
	defer timer.Stop()
	var sig *signaling.Client
	if address, direct := signaling.DirectAddress(room); direct {
		sig, err = signaling.DialDirect(ctx, address, raw)
	} else {
		sig, err = signaling.Dial(ctx, url, room, signaling.RoleViewer, raw)
	}
	if err != nil {
		cancel()
		return err
	}
	p, err := p2p.NewViewer(ctx, sig, func(f p2p.Frame) { v.enqueueSessionFrame(f, generation) }, func(c p2p.Control) { v.receiveStreamControl(c, generation) })
	if err != nil {
		sig.Close()
		cancel()
		return err
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	// 連線完成的判斷只能依賴新版雙端共用的內建能力；桌面模式的 Host
	// 可能沒有註冊 terminal.open，否則 GUI 會在已完成 P2P/ICE 後永遠等待。
	for !p.SupportsCommand("capabilities.get") {
		select {
		case <-ctx.Done():
			p.Close()
			sig.Close()
			return fmt.Errorf("連線逾時或已取消")
		case <-p.Done():
			sig.Close()
			cancel()
			return fmt.Errorf("遠端已斷線")
		case <-ticker.C:
		}
	}
	v.mu.Lock()
	if ctx.Err() != nil || v.generation != generation {
		v.mu.Unlock()
		p.Close()
		sig.Close()
		return context.Canceled
	}
	v.peer = p
	v.mu.Unlock()
	go func() {
		select {
		case <-ctx.Done():
			p.Close()
		case <-p.Done():
			cancel()
		}
		sig.Close()
	}()
	return nil
}

func (v *Viewer) beginConnect() (context.Context, context.CancelFunc, uint64) {
	ctx, cancel := context.WithCancel(context.Background())
	v.mu.Lock()
	p, c := v.peer, v.cancel
	v.generation++
	generation := v.generation
	v.peer, v.cancel = nil, cancel
	v.resetStreamLocked()
	v.frameMu.Lock()
	v.resetFramesLocked(generation)
	v.frameMu.Unlock()
	v.mu.Unlock()
	if c != nil {
		c()
	}
	if p != nil {
		_ = p.Close()
	}
	v.state("closed")
	return ctx, cancel, generation
}

func (v *Viewer) Close() {
	v.mu.Lock()
	p, c := v.peer, v.cancel
	v.generation++
	v.peer = nil
	v.cancel = nil
	v.resetStreamLocked()
	v.frameMu.Lock()
	v.resetFramesLocked(v.generation)
	v.frameMu.Unlock()
	v.mu.Unlock()
	if c != nil {
		c()
	}
	if p != nil {
		_ = p.Close()
	}
	v.state("closed")
}
func (v *Viewer) Terminal() *TerminalSession { return &TerminalSession{viewer: v} }

const (
	maxQueuedFrames     = 512
	maxQueuedFrameBytes = 32 * 1024 * 1024
)

func (v *Viewer) resetFramesLocked(generation uint64) {
	v.frames = nil
	v.frameBytes = 0
	v.frameGeneration = generation
	v.awaitingKeyframe = true
	v.recoveryPending = false
}

// enqueueFrame 保留接收順序。JPEG 差分影格的 x/y 是貼回完整畫面的座標，
// 少掉任何一個區塊都可能留下舊畫面。溢位時丟棄整條相依鏈，直到完整
// keyframe 才恢復；張數與位元組都設上限，不在 DataChannel 回呼等待解碼。
func (v *Viewer) enqueueFrame(f p2p.Frame) {
	v.enqueueSessionFrame(f, 0)
}

func (v *Viewer) enqueueSessionFrame(f p2p.Frame, generation uint64) {
	copyFrame := f
	v.frameMu.Lock()
	if generation != v.frameGeneration {
		v.frameMu.Unlock()
		return
	}
	if len(f.JPEG) == 0 || len(f.JPEG) > maxQueuedFrameBytes || len(v.frames) >= maxQueuedFrames || v.frameBytes > maxQueuedFrameBytes-len(f.JPEG) {
		v.frames = nil
		v.frameBytes = 0
		v.awaitingKeyframe = true
		v.recoveryPending = true
	}
	if len(f.JPEG) == 0 || len(f.JPEG) > maxQueuedFrameBytes || (v.awaitingKeyframe && !f.Keyframe) {
		v.recoveryPending = true
		v.frameMu.Unlock()
		return
	}
	if f.Keyframe {
		v.awaitingKeyframe = false
		v.recoveryPending = false
	}
	v.frames = append(v.frames, &copyFrame)
	v.frameBytes += len(f.JPEG)
	v.frameMu.Unlock()

	v.mu.Lock()
	h := v.onFrame
	current := v.generation == generation
	v.mu.Unlock()
	if current && h != nil {
		h(FrameInfo{Sequence: f.Sequence, Display: f.Display, Width: int(f.Width), Height: int(f.Height), Codec: int(f.Codec), Keyframe: f.Keyframe, Bytes: len(f.JPEG)})
	}
}

// ConsumeFrameRecoveryRequest 由 Android frame pump 輪詢，並在自己的
// command worker 節流要求 video.keyframe；接收回呼只設旗標，不等待網路。
func (v *Viewer) ConsumeFrameRecoveryRequest() bool {
	v.frameMu.Lock()
	defer v.frameMu.Unlock()
	pending := v.recoveryPending
	v.recoveryPending = false
	return pending
}

// ReadFrameJSON 依接收順序取得一筆影格。JPEG 差分影格必須逐筆合成，
// 所以不能覆寫成「最近一筆」；尚未有影格時回傳空字串。
func (v *Viewer) ReadFrameJSON() string {
	f := v.popFrame()
	if f == nil {
		return ""
	}
	b, _ := json.Marshal(struct {
		Sequence uint64 `json:"sequence"`
		Display  int    `json:"display"`
		Width    uint32 `json:"width"`
		Height   uint32 `json:"height"`
		X        uint32 `json:"x"`
		Y        uint32 `json:"y"`
		Codec    byte   `json:"codec"`
		Keyframe bool   `json:"keyframe"`
		Data     []byte `json:"data"`
	}{f.Sequence, f.Display, f.Width, f.Height, f.X, f.Y, f.Codec, f.Keyframe, f.JPEG})
	return string(b)
}

func (v *Viewer) popFrame() *p2p.Frame {
	v.frameMu.Lock()
	if len(v.frames) == 0 {
		v.frameMu.Unlock()
		return nil
	}
	f := v.frames[0]
	copy(v.frames, v.frames[1:])
	v.frames[len(v.frames)-1] = nil
	v.frames = v.frames[:len(v.frames)-1]
	v.frameBytes -= len(f.JPEG)
	// JSON/Base64 costs scale with payload size. Never hold the receive lock
	// while serializing or copying the return string across the JNI boundary.
	v.frameMu.Unlock()
	return f
}

// TrafficJSON 回傳目前 P2P DataChannel 的累計有效資料量。
// Android 端以兩次取樣差值計算 TX/RX 每秒速率，與 PC 標題列相同。
func (v *Viewer) TrafficJSON() string {
	v.mu.Lock()
	p := v.peer
	v.mu.Unlock()
	if p == nil {
		return `{"sentBytes":0,"receivedBytes":0}`
	}
	sent, received := p.TrafficBytes()
	b, _ := json.Marshal(struct {
		Sent     uint64 `json:"sentBytes"`
		Received uint64 `json:"receivedBytes"`
	}{sent, received})
	return string(b)
}

// SupportsCommand 與 CallCommand 供 Android 在硬體解碼器重建後要求 Host
// 立即送出 IDR；舊版 Host 不支援時由呼叫端忽略錯誤並等待週期性 keyframe。
func (v *Viewer) SupportsCommand(method string) bool {
	v.mu.Lock()
	p := v.peer
	v.mu.Unlock()
	return p != nil && p.SupportsCommand(method)
}

func (v *Viewer) CallCommand(method string) error {
	v.mu.Lock()
	p := v.peer
	v.mu.Unlock()
	if p == nil {
		return fmt.Errorf("遠端尚未連線")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := p.CallCommand(ctx, method)
	return err
}

type TerminalSession struct {
	viewer   *Viewer
	mu       sync.Mutex
	open     bool
	seq, ack uint64
}
type TerminalReadResult struct {
	Sequence uint64
	Data     []byte
	Ended    bool
	Error    string
}

func (t *TerminalSession) call(method string, params any) error {
	_, e := t.callResult(method, params)
	return e
}
func (t *TerminalSession) callResult(method string, params any) (p2p.CommandResponse, error) {
	t.viewer.mu.Lock()
	p := t.viewer.peer
	t.viewer.mu.Unlock()
	if p == nil {
		return p2p.CommandResponse{}, fmt.Errorf("遠端尚未連線")
	}
	ctx, c := context.WithTimeout(context.Background(), 10*time.Second)
	defer c()
	return p.CallCommandParams(ctx, method, mustJSON(params))
}
func (t *TerminalSession) Open(c, r int) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if c < 20 || c > 500 || r < 5 || r > 300 {
		return fmt.Errorf("終端機大小無效")
	}
	err := t.call("terminal.open", map[string]any{"columns": c, "rows": r})
	t.open = err == nil
	return err
}
func (t *TerminalSession) Write(data []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.open {
		return fmt.Errorf("終端機尚未開啟")
	}
	t.seq++
	return t.call("terminal.write", map[string]any{"data": data, "sequence": t.seq})
}
func (t *TerminalSession) Resize(c, r int) error {
	return t.call("terminal.resize", map[string]any{"columns": c, "rows": r})
}

// 使用 gomobile 支援的 int64；保留 Host 的完整 JSON 結果。
func (t *TerminalSession) ReadJSON(ack int64) (string, error) {
	if ack < 0 {
		return "", fmt.Errorf("輸出序號無效")
	}
	r, e := t.callResult("terminal.read", map[string]any{"ack": ack})
	if e != nil {
		return "", e
	}
	return string(r.Result), nil
}
func (t *TerminalSession) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.open {
		return nil
	}
	t.open = false
	return t.call("terminal.close", nil)
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

// SendVideoCapabilities 讓 Android Viewer 在通道建立後公告可解碼的 wire codec。
// codec 0 是 JPEG、1 是 H.264、2 是 HEVC；hardware 只列出已通過 MediaCodec
// 建立測試的硬體候選，Host 會依此選擇編碼器。
func (v *Viewer) SendVideoCapabilities(codecs, hardware []byte) error {
	v.mu.Lock()
	p := v.peer
	v.mu.Unlock()
	if p == nil {
		return fmt.Errorf("遠端尚未連線")
	}
	return p.SendControl(p2p.Control{
		Type:                 "video-capabilities",
		Codecs:               append([]byte(nil), codecs...),
		HardwareDecodeCodecs: append([]byte(nil), hardware...),
		KeyframeInterval:     10,
	})
}

// SendControlJSON 傳送既有 control DataChannel JSON 封包。
func (v *Viewer) SendControlJSON(payload string) error {
	if !utf8.ValidString(payload) || len(payload) > 128*1024 {
		return fmt.Errorf("控制封包不是有效 UTF-8 或超過大小上限")
	}
	v.mu.Lock()
	p := v.peer
	v.mu.Unlock()
	if p == nil {
		return fmt.Errorf("遠端尚未連線")
	}
	var c p2p.Control
	if err := json.Unmarshal([]byte(payload), &c); err != nil {
		return err
	}
	if c.Type == "text" && (len(c.Text) == 0 || len(c.Text) > 16*1024 || !utf8.ValidString(c.Text)) {
		return fmt.Errorf("輸入文字不是有效 UTF-8 或超過 16 KiB")
	}
	return p.SendControl(c)
}
