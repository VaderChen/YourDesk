package p2p

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
	"yourdesk/internal/idleclose"

	"github.com/pion/transport/v4"
	"github.com/pion/transport/v4/stdnet"
	"github.com/pion/webrtc/v4"
)

// 有回覆但命令未執行（時鐘誤差／忙碌）不代表通道已死亡。
func TestIdleConnectionWithRejectedHeartbeat(t *testing.T) {
	for _, code := range []string{"busy", "expired", "invalid_request", "expired_or_invalid", "in_progress", "unsupported_method"} {
		t.Run(code, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				var closed atomic.Bool
				var idle idleclose.Monitor
				now := time.Now()
				idle.Expired(now, false, true)
				go monitorLiveness(ctx, func() bool { return true }, func(context.Context) error {
					r := CommandResponse{Version: CommandVersion, ID: 1, Code: code, Error: "remote rejection"}
					return livenessResult(r, commandResponseError(r))
				}, func() { closed.Store(true) }, func() bool { return false })
				time.Sleep(time.Hour)
				synctest.Wait()
				if closed.Load() || idle.Expired(time.Now(), false, true) {
					t.Fatal("閒置關閉未啟用且心跳收到回覆，仍自行斷線")
				}
			})
		})
	}
}

func TestLivenessRejectionsAcrossDataChannel(t *testing.T) {
	a, b := clipboardTestPeers(t, func(Control) {})
	for _, tc := range []struct {
		offset time.Duration
		code   string
	}{{-time.Second, "expired"}, {time.Minute, "invalid_request"}} {
		b.mu.RLock()
		dc := b.control
		b.mu.RUnlock()
		b.onMessage(dc, func(m webrtc.DataChannelMessage) {
			var c Control
			if decodeControl(m.Data, &c) == nil {
				if c.CommandRequest != nil {
					// 模擬雙端時鐘不同，不改動本機系統時間或正式授權規則。
					c.CommandRequest.TimeoutMS = nil // 驗證舊版絕對期限的拒絕仍不等於斷線。
					c.CommandRequest.Expires = time.Now().Add(tc.offset).UnixMilli()
				}
				b.handleCommand(c)
			}
		})
		response, err := a.CallCommand(context.Background(), "ping")
		if response.Code != tc.code || err == nil {
			t.Fatalf("指令本身仍須回報拒絕：%+v %v", response, err)
		}
		if livenessResult(response, err) != nil {
			t.Fatalf("已完成實際 DataChannel 往返仍被視為無回應：%+v %v", response, err)
		}
	}
}

func TestDisconnectReasonKeepsFirstCause(t *testing.T) {
	p := &Peer{done: make(chan struct{})}
	p.markClosedReason(pingFailureMessage)
	p.markClosedReason(disconnectedMessage)
	p.markClosed()
	if p.DisconnectReason() != pingFailureMessage {
		t.Fatal("後續清理覆蓋斷線原因")
	}
	p = &Peer{done: make(chan struct{})}
	p.markClosed()
	p.markClosedReason(disconnectedMessage)
	if p.DisconnectReason() != "" {
		t.Fatal("主動關閉產生錯誤提示")
	}
}

type interruptibleNet struct {
	transport.Net
	blocked atomic.Bool
}

func (n *interruptibleNet) ListenUDP(network string, addr *net.UDPAddr) (transport.UDPConn, error) {
	conn, err := n.Net.ListenUDP(network, addr)
	if err != nil {
		return nil, err
	}
	return &interruptibleUDP{conn, &n.blocked}, nil
}

type interruptibleUDP struct {
	transport.UDPConn
	blocked *atomic.Bool
}

func (c *interruptibleUDP) ReadFrom(b []byte) (int, net.Addr, error) {
	for {
		n, addr, err := c.UDPConn.ReadFrom(b)
		if err != nil || !c.blocked.Load() {
			return n, addr, err
		}
	}
}

func (c *interruptibleUDP) WriteTo(b []byte, addr net.Addr) (int, error) {
	if c.blocked.Load() {
		return len(b), nil
	}
	return c.UDPConn.WriteTo(b, addr)
}

// 使用真正的 ICE／DTLS／SCTP，縮短 ICE 時序以測試斷網後的自動恢復。
func TestIdleConnectionRecoversTemporaryNetworkLoss(t *testing.T) {
	t.Setenv("YOURDESK_AUTH_LOG_DIR", t.TempDir())
	create := func() (*Peer, *interruptibleNet) {
		n, err := stdnet.NewNet()
		if err != nil {
			t.Fatal(err)
		}
		network := &interruptibleNet{Net: n}
		settings := webrtc.SettingEngine{}
		settings.SetNet(network)
		settings.SetICETimeouts(250*time.Millisecond, 3*time.Second, 50*time.Millisecond)
		pc, err := webrtc.NewAPI(webrtc.WithSettingEngine(settings)).NewPeerConnection(webrtc.Configuration{})
		if err != nil {
			t.Fatal(err)
		}
		p := &Peer{pc: pc, done: make(chan struct{})}
		p.bindConnectionStates()
		t.Cleanup(func() { _ = p.Close() })
		return p, network
	}
	a, network := create()
	b, _ := create()
	bind := func(p *Peer, dc *webrtc.DataChannel) {
		p.mu.Lock()
		p.control = dc
		p.mu.Unlock()
		p.onMessage(dc, func(m webrtc.DataChannelMessage) {
			var c Control
			if decodeControl(m.Data, &c) == nil {
				p.handleCommand(c)
			}
		})
		dc.OnOpen(func() { p.announceCommands() })
	}
	b.pc.OnDataChannel(func(dc *webrtc.DataChannel) { bind(b, dc) })
	dc, err := a.pc.CreateDataChannel(ControlChannel, nil)
	if err != nil {
		t.Fatal(err)
	}
	bind(a, dc)
	setLocal := func(p *Peer, d webrtc.SessionDescription) {
		gathered := webrtc.GatheringCompletePromise(p.pc)
		if err := p.pc.SetLocalDescription(d); err != nil {
			t.Fatal(err)
		}
		select {
		case <-gathered:
		case <-time.After(5 * time.Second):
			t.Fatal("ICE gathering timeout")
		}
	}
	offer, err := a.pc.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	setLocal(a, offer)
	if err := b.pc.SetRemoteDescription(*a.pc.LocalDescription()); err != nil {
		t.Fatal(err)
	}
	answer, err := b.pc.CreateAnswer(nil)
	if err != nil {
		t.Fatal(err)
	}
	setLocal(b, answer)
	if err := a.pc.SetRemoteDescription(*b.pc.LocalDescription()); err != nil {
		t.Fatal(err)
	}
	wait := func(ready func() bool) {
		until := time.Now().Add(5 * time.Second)
		for !ready() {
			if time.Now().After(until) {
				t.Fatalf("ICE 狀態等待逾時：%v／%v", a.pc.ICEConnectionState(), b.pc.ICEConnectionState())
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	wait(func() bool { return a.SupportsCommand("ping") && b.SupportsCommand("ping") })
	if _, err := a.CallCommand(context.Background(), "ping"); err != nil {
		t.Fatal(err)
	}
	network.blocked.Store(true)
	wait(func() bool {
		return a.pc.ICEConnectionState() == webrtc.ICEConnectionStateDisconnected ||
			b.pc.ICEConnectionState() == webrtc.ICEConnectionStateDisconnected ||
			a.pc.ConnectionState() == webrtc.PeerConnectionStateClosed || b.pc.ConnectionState() == webrtc.PeerConnectionStateClosed
	})
	for _, p := range []*Peer{a, b} {
		select {
		case <-p.Done():
			t.Fatal("可恢復的暫時 ICE 中斷被立即關閉")
		default:
		}
	}
	network.blocked.Store(false)
	wait(func() bool { return a.Connected() && b.Connected() })
	if _, err := a.CallCommand(context.Background(), "ping"); err != nil {
		t.Fatal("網路恢復後控制通道無法往返：", err)
	}
	// 完全失聯仍須結束，不能把修正變成永久不偵測故障。
	network.blocked.Store(true)
	select {
	case <-a.Done():
	case <-b.Done():
	case <-time.After(6 * time.Second):
		t.Fatal("持續失聯未結束")
	}
}
