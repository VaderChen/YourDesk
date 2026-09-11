package p2p

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
	"yourdesk/internal/rawkey"
	"yourdesk/internal/signaling"
	"yourdesk/internal/streamconfig"
)

const (
	ScreenChannel    = "screen"
	ControlChannel   = "control"
	ClipboardChannel = "clipboard"
	chunkSize        = 28 * 1024
	frameHeaderSize  = 44
	maxScreenBuffer  = 2 * 1024 * 1024
)

type Frame struct {
	Display  int  // -1 表示舊版未提供螢幕資訊。
	Codec    byte // 0=JPEG、1=H.264、2=HEVC；沿用保留的標頭位元組。
	Sequence uint64
	Width    uint32
	Height   uint32
	X        uint32
	Y        uint32
	Keyframe bool
	JPEG     []byte
}

// EnhancementReport 回報來源實際使用的尺寸及影片目標碼率；零碼率表示品質控制。
type EnhancementReport struct {
	ConfigRevision uint64 `json:"configRevision,omitempty"`
	Enabled        bool   `json:"enabled"`
	SourceWidth    int    `json:"sourceWidth"`
	SourceHeight   int    `json:"sourceHeight"`
	StreamWidth    int    `json:"streamWidth"`
	StreamHeight   int    `json:"streamHeight"`
	Bitrate        int    `json:"bitrate"`
}
type Control struct {
	AppVersion           string                     `json:"appVersion,omitempty"`
	KeyframeInterval     int                        `json:"keyframeInterval,omitempty"`
	StreamConfig         *streamconfig.Request      `json:"streamConfig,omitempty"`
	StreamCapabilities   *streamconfig.Capabilities `json:"streamCapabilities,omitempty"`
	StreamResult         *streamconfig.Result       `json:"streamResult,omitempty"`
	EnhancementReport    *EnhancementReport         `json:"enhancementReport,omitempty"`
	ImageEnhancement     bool                       `json:"imageEnhancement,omitempty"`
	EnhancementSupported bool                       `json:"enhancementSupported,omitempty"`
	DisableMapping       bool                       `json:"disableMapping,omitempty"`
	Platform             string                     `json:"platform,omitempty"`
	VideoCodec           byte                       `json:"videoCodec,omitempty"`
	EncodingMode         string                     `json:"encodingMode,omitempty"`
	RawKey               *rawkey.Event              `json:"rawKey,omitempty"`
	Profile              string                     `json:"profile,omitempty"`
	ViewWidth            int                        `json:"viewWidth,omitempty"`
	ViewHeight           int                        `json:"viewHeight,omitempty"`
	Clipboard            []byte                     `json:"clipboard,omitempty"`
	DisplayRequest       uint64                     `json:"displayRequest,omitempty"`
	Display              *int                       `json:"display,omitempty"`
	DisplayCount         int                        `json:"displayCount,omitempty"`
	Codecs               []byte                     `json:"codecs,omitempty"`
	Type                 string                     `json:"type"`
	X                    float64                    `json:"x,omitempty"`
	Y                    float64                    `json:"y,omitempty"`
	Button               int                        `json:"button,omitempty"`
	Down                 bool                       `json:"down,omitempty"`
	Key                  string                     `json:"key,omitempty"`
	Delta                float64                    `json:"delta,omitempty"`
}

type Peer struct {
	pc             *webrtc.PeerConnection
	screen         *webrtc.DataChannel
	control        *webrtc.DataChannel
	clipboard      *webrtc.DataChannel
	clipboardInbox chan []byte
	clipboardDone  chan struct{}
	clipboardOnce  sync.Once
	mu             sync.RWMutex
	closed         bool
	done           chan struct{}
	doneOnce       sync.Once
	controlMu      sync.Mutex
	pendingControl [][]byte
}

func config() webrtc.Configuration {
	return webrtc.Configuration{
		ICEServers:         []webrtc.ICEServer{{URLs: []string{"stun:stun.l.google.com:19302", "stun:stun.cloudflare.com:3478"}}},
		ICETransportPolicy: webrtc.ICETransportPolicyAll,
	}
}

func NewHost(ctx context.Context, signal *signaling.Client, onControl func(Control)) (*Peer, error) {
	pc, err := webrtc.NewPeerConnection(connectionConfig(signal))
	if err != nil {
		return nil, err
	}
	p := &Peer{pc: pc, done: make(chan struct{}), clipboardInbox: make(chan []byte, 128), clipboardDone: make(chan struct{})}
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateClosed || state == webrtc.PeerConnectionStateFailed {
			p.markClosed()
		}
	})
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c != nil && strings.Contains(c.ToJSON().Candidate, " typ relay ") {
			_ = pc.Close()
		}
	})
	pc.OnICEConnectionStateChange(func(s webrtc.ICEConnectionState) {
		if s == webrtc.ICEConnectionStateFailed || s == webrtc.ICEConnectionStateDisconnected {
			_ = pc.Close()
		}
	})
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		switch dc.Label() {
		case ClipboardChannel:
			p.bindClipboard(dc)
		case ScreenChannel:
			p.mu.Lock()
			p.screen = dc
			p.mu.Unlock()
		case ControlChannel:
			p.mu.Lock()
			p.control = dc
			p.mu.Unlock()
			dc.OnOpen(func() { p.flushControl() })
			dc.OnMessage(func(m webrtc.DataChannelMessage) {
				var c Control
				if decodeControl(m.Data, &c) == nil && onControl != nil {
					onControl(c)
				}
			})
		}
	})
	// Host creates channels so their semantics are explicit on both peers.
	if p.screen, err = pc.CreateDataChannel(ScreenChannel, &webrtc.DataChannelInit{Ordered: boolPtr(false), MaxRetransmits: uint16Ptr(0)}); err != nil {
		_ = pc.Close()
		return nil, err
	}
	if p.control, err = pc.CreateDataChannel(ControlChannel, &webrtc.DataChannelInit{Ordered: boolPtr(true)}); err != nil {
		_ = pc.Close()
		return nil, err
	}
	dc, err := pc.CreateDataChannel(ClipboardChannel, &webrtc.DataChannelInit{Ordered: boolPtr(true)})
	if err != nil {
		_ = pc.Close()
		return nil, err
	}
	p.bindClipboard(dc)
	// This channel is created locally by the host, so OnDataChannel is not
	// invoked for it. Bind the receive callback explicitly.
	p.control.OnMessage(func(m webrtc.DataChannelMessage) {
		var c Control
		if decodeControl(m.Data, &c) == nil && onControl != nil {
			onControl(c)
		}
	})
	p.control.OnOpen(func() { p.flushControl() })
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		_ = pc.Close()
		return nil, err
	}
	if err = pc.SetLocalDescription(offer); err != nil {
		_ = pc.Close()
		return nil, err
	}
	if err = waitGather(ctx, pc); err != nil {
		_ = pc.Close()
		return nil, err
	}
	if err := rejectRelay(pc.LocalDescription().SDP); err != nil {
		_ = pc.Close()
		return nil, err
	}
	e, err := signal.OfferAndReceive(ctx, pc.LocalDescription())
	if err != nil {
		_ = pc.Close()
		return nil, err
	}
	if e.Kind != signaling.KindAnswer {
		_ = pc.Close()
		return nil, fmt.Errorf("預期 answer，收到 %s", e.Kind)
	}
	var answer webrtc.SessionDescription
	if err := e.DecodePayload(&answer); err != nil {
		_ = pc.Close()
		return nil, err
	}
	if err := rejectRelay(answer.SDP); err != nil {
		_ = pc.Close()
		return nil, err
	}
	if err := pc.SetRemoteDescription(answer); err != nil {
		_ = pc.Close()
		return nil, err
	}
	return p, nil
}

func NewViewer(ctx context.Context, signal *signaling.Client, onFrame func(Frame), onControl ...func(Control)) (*Peer, error) {
	pc, err := webrtc.NewPeerConnection(connectionConfig(signal))
	if err != nil {
		return nil, err
	}
	p := &Peer{pc: pc, done: make(chan struct{}), clipboardInbox: make(chan []byte, 128), clipboardDone: make(chan struct{})}
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateClosed || state == webrtc.PeerConnectionStateFailed {
			p.markClosed()
		}
	})
	pc.OnICEConnectionStateChange(func(s webrtc.ICEConnectionState) {
		if s == webrtc.ICEConnectionStateFailed || s == webrtc.ICEConnectionStateDisconnected {
			_ = pc.Close()
		}
	})
	var assembler frameAssembler
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		switch dc.Label() {
		case ClipboardChannel:
			p.bindClipboard(dc)
		case ScreenChannel:
			p.mu.Lock()
			p.screen = dc
			p.mu.Unlock()
			dc.OnMessage(func(m webrtc.DataChannelMessage) {
				if f, ok := assembler.add(m.Data); ok && onFrame != nil {
					onFrame(f)
				}
			})
		case ControlChannel:
			p.mu.Lock()
			p.control = dc
			p.mu.Unlock()
			dc.OnOpen(func() { p.flushControl() })
			dc.OnMessage(func(m webrtc.DataChannelMessage) {
				var c Control
				if decodeControl(m.Data, &c) == nil {
					for _, handler := range onControl {
						if handler != nil {
							handler(c)
						}
					}
				}
			})
		}
	})
	e, err := signal.Receive(ctx)
	if err != nil {
		_ = pc.Close()
		return nil, err
	}
	if e.Kind != signaling.KindOffer {
		_ = pc.Close()
		return nil, fmt.Errorf("預期 offer，收到 %s", e.Kind)
	}
	var offerRemote webrtc.SessionDescription
	if err := e.DecodePayload(&offerRemote); err != nil {
		_ = pc.Close()
		return nil, err
	}
	if err := rejectRelay(offerRemote.SDP); err != nil {
		_ = pc.Close()
		return nil, err
	}
	if err := pc.SetRemoteDescription(offerRemote); err != nil {
		_ = pc.Close()
		return nil, err
	}
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		_ = pc.Close()
		return nil, err
	}
	if err = pc.SetLocalDescription(answer); err != nil {
		_ = pc.Close()
		return nil, err
	}
	if err = waitGather(ctx, pc); err != nil {
		_ = pc.Close()
		return nil, err
	}
	if err := rejectRelay(pc.LocalDescription().SDP); err != nil {
		_ = pc.Close()
		return nil, err
	}
	if err := signal.Send(ctx, signaling.KindAnswer, pc.LocalDescription()); err != nil {
		_ = pc.Close()
		return nil, err
	}
	return p, nil
}

var ErrFrameDropped = errors.New("畫面傳輸壅塞，影格已捨棄")

func (p *Peer) SendFrame(f Frame) error {
	err := p.SendFrameChecked(f)
	if f.Codec == 0 && errors.Is(err, ErrFrameDropped) {
		return nil
	}
	return err
}

// SendFrameChecked 對 JPEG 也回報丟幀，供管線重建差異基底。
func (p *Peer) SendFrameChecked(f Frame) error {
	p.mu.RLock()
	dc := p.screen
	closed := p.closed
	p.mu.RUnlock()
	if closed || dc == nil {
		return errors.New("screen channel 尚未連線")
	}
	if len(f.JPEG) == 0 {
		return errors.New("空白影格")
	}
	if dc.ReadyState() != webrtc.DataChannelStateOpen || dc.BufferedAmount() > maxScreenBuffer {
		return ErrFrameDropped
	}
	total := (len(f.JPEG) + chunkSize - 1) / chunkSize
	for i := 0; i < total; i++ {
		start, end := i*chunkSize, (i+1)*chunkSize
		if end > len(f.JPEG) {
			end = len(f.JPEG)
		}
		msg := make([]byte, frameHeaderSize+end-start)
		binary.BigEndian.PutUint64(msg[0:8], f.Sequence)
		binary.BigEndian.PutUint32(msg[8:12], f.Width)
		binary.BigEndian.PutUint32(msg[12:16], f.Height)
		binary.BigEndian.PutUint32(msg[16:20], uint32(i))
		binary.BigEndian.PutUint32(msg[20:24], uint32(total))
		binary.BigEndian.PutUint64(msg[24:32], uint64(time.Now().UnixNano()))
		binary.BigEndian.PutUint32(msg[32:36], f.X)
		binary.BigEndian.PutUint32(msg[36:40], f.Y)
		if f.Keyframe {
			msg[40] = 1
		}
		msg[41] = f.Codec
		binary.BigEndian.PutUint16(msg[42:44], uint16(f.Display+1))
		copy(msg[frameHeaderSize:], f.JPEG[start:end])
		if err := dc.Send(msg); err != nil {
			return err
		}
	}
	return nil
}

func (p *Peer) SendControl(c Control) error {
	p.mu.RLock()
	dc := p.control
	closed := p.closed
	p.mu.RUnlock()
	if closed || dc == nil {
		return errors.New("control channel 尚未連線")
	}
	b, err := encodeControl(c)
	if err != nil {
		return err
	}
	if dc.ReadyState() != webrtc.DataChannelStateOpen {
		p.controlMu.Lock()
		if len(p.pendingControl) >= 64 {
			p.pendingControl = p.pendingControl[len(p.pendingControl)-63:]
		}
		p.pendingControl = append(p.pendingControl, b)
		p.controlMu.Unlock()
		return nil
	}
	return dc.Send(b)
}

func (p *Peer) flushControl() {
	p.mu.RLock()
	dc := p.control
	p.mu.RUnlock()
	if dc == nil || dc.ReadyState() != webrtc.DataChannelStateOpen {
		return
	}
	p.controlMu.Lock()
	queued := p.pendingControl
	p.pendingControl = nil
	p.controlMu.Unlock()
	for _, b := range queued {
		_ = dc.Send(b)
	}
}

// Done 在 P2P 工作階段失敗或結束時關閉，供上層統一處理生命週期。
func (p *Peer) Done() <-chan struct{} { return p.done }
func (p *Peer) markClosed() {
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()
	p.doneOnce.Do(func() { close(p.done) })
}
func (p *Peer) Close() error { p.markClosed(); return p.pc.Close() }

func waitGather(ctx context.Context, pc *webrtc.PeerConnection) error {
	if pc.ICEGatheringState() == webrtc.ICEGatheringStateComplete {
		return nil
	}
	t := time.NewTicker(50 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			if pc.ICEGatheringState() == webrtc.ICEGatheringStateComplete {
				return nil
			}
		}
	}
}

func rejectRelay(sdp string) error {
	for _, line := range strings.Split(sdp, "\n") {
		if strings.Contains(line, " typ relay") {
			return errors.New("偵測到 relay candidate；YourDesk 僅允許 direct UDP P2P")
		}
	}
	return nil
}

func boolPtr(v bool) *bool       { return &v }
func uint16Ptr(v uint16) *uint16 { return &v }

// 區域網路直連不依赖外部 STUN，離線也可完成 ICE。
func connectionConfig(signal *signaling.Client) webrtc.Configuration {
	if signal.IsDirect() {
		return webrtc.Configuration{}
	}
	return config()
}

// TrafficBytes 回傳此連線影像、控制與剪貼簿通道的累計有效資料量。
// 不包含網路封包標頭、重傳與其他程式的流量。
func (p *Peer) TrafficBytes() (sent, received uint64) {
	for _, stats := range p.pc.GetStats() {
		if channel, ok := stats.(webrtc.DataChannelStats); ok {
			sent += channel.BytesSent
			received += channel.BytesReceived
		}
	}
	return
}

// RoundTripMS 只採用被選用的 ICE 連線；沒有有效量測時保留未知。
func (p *Peer) RoundTripMS() *float64 {
	for _, stat := range p.pc.GetStats() {
		if pair, ok := stat.(webrtc.ICECandidatePairStats); ok && pair.Nominated && pair.State == webrtc.StatsICECandidatePairStateSucceeded && pair.CurrentRoundTripTime > 0 {
			ms := pair.CurrentRoundTripTime * 1000
			return &ms
		}
	}
	return nil
}

func (p *Peer) Connected() bool { return p.pc.ConnectionState() == webrtc.PeerConnectionStateConnected }
