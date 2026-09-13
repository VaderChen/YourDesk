package p2p

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/pion/webrtc/v4"
	"testing"
	"time"
	"yourdesk/internal/authlog"
)

func TestPacketProbeIntegrityAndGate(t *testing.T) {
	t.Setenv("YOURDESK_AUTH_LOG_DIR", t.TempDir())
	data := bytes.Repeat([]byte{37}, 4096)
	raw, _ := json.Marshal(probePayload{data, probeHash(data)})
	if _, err := answerProbe(raw); err == nil {
		t.Fatal("disabled remote accepted probe")
	}
	if err := authlog.SetDebug(true); err != nil {
		t.Fatal(err)
	}
	result, err := answerProbe(raw)
	if err != nil {
		t.Fatal(err)
	}
	reply := result.(probeReply)
	if reply.Received != 4096 || reply.ReceivedHash != probeHash(data) || len(reply.Data) != 4096 || reply.Hash != probeHash(reply.Data) {
		t.Fatal("bad bidirectional checksum")
	}
	data[0]++
	bad, _ := json.Marshal(probePayload{data, reply.ReceivedHash})
	if _, err := answerProbe(bad); err == nil {
		t.Fatal("corruption accepted")
	}
	oversized := bytes.Repeat([]byte{1}, 4097)
	bad, _ = json.Marshal(probePayload{oversized, probeHash(oversized)})
	if _, err := answerProbe(bad); err == nil {
		t.Fatal("oversized probe accepted")
	}
}

// 真正經過本機 ICE／DTLS／SCTP／DataChannel，驗證分片後的雙向資料。
func TestPacketProbeWebRTCSmoke(t *testing.T) {
	t.Setenv("YOURDESK_AUTH_LOG_DIR", t.TempDir())
	if err := authlog.SetDebug(true); err != nil {
		t.Fatal(err)
	}
	a, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	pa := &Peer{pc: a, done: make(chan struct{})}
	pb := &Peer{pc: b, done: make(chan struct{})}
	ready := make(chan struct{}, 2)
	bind := func(p *Peer, dc *webrtc.DataChannel) {
		p.mu.Lock()
		p.control = dc
		p.mu.Unlock()
		dc.OnMessage(func(m webrtc.DataChannelMessage) {
			var c Control
			if decodeControl(m.Data, &c) == nil {
				p.handleCommand(c)
			}
		})
		dc.OnOpen(func() { ready <- struct{}{} })
	}
	b.OnDataChannel(func(dc *webrtc.DataChannel) { bind(pb, dc) })
	dc, err := a.CreateDataChannel(ControlChannel, nil)
	if err != nil {
		t.Fatal(err)
	}
	bind(pa, dc)
	offer, err := a.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	g := webrtc.GatheringCompletePromise(a)
	if err = a.SetLocalDescription(offer); err != nil {
		t.Fatal(err)
	}
	<-g
	if err = b.SetRemoteDescription(*a.LocalDescription()); err != nil {
		t.Fatal(err)
	}
	answer, err := b.CreateAnswer(nil)
	if err != nil {
		t.Fatal(err)
	}
	g = webrtc.GatheringCompletePromise(b)
	if err = b.SetLocalDescription(answer); err != nil {
		t.Fatal(err)
	}
	<-g
	if err = a.SetRemoteDescription(*b.LocalDescription()); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-ready:
		case <-time.After(10 * time.Second):
			t.Fatal("channel not ready")
		}
	}
	pa.commandsInit()
	pa.commands.remote = pb.localCommands()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r := pa.RunPacketTest(ctx)
	if r.Error != "" || r.Verified < 3 || r.Sent != r.Received {
		t.Fatalf("probe failed: %+v", r)
	}
}
