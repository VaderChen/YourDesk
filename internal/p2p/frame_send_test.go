package p2p

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

func frameSendTestPeer(t testing.TB, receive func([]byte)) *Peer {
	t.Helper()
	a, b := clipboardTestPeers(t, nil)
	b.pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		dc.OnMessage(func(m webrtc.DataChannelMessage) { receive(m.Data) })
	})
	dc, err := a.pc.CreateDataChannel(ScreenChannel, nil)
	if err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	a.screen = dc
	a.mu.Unlock()
	clipboardEventually(t, func() bool { return dc.ReadyState() == webrtc.DataChannelStateOpen })
	return a
}

func TestSendFrameFragmentOwnership(t *testing.T) {
	packets := make(chan []byte, 128)
	p := frameSendTestPeer(t, func(data []byte) { packets <- data })
	for i, size := range []int{1, chunkSize, chunkSize + 1, 4*chunkSize + 137, chunkSize + 11} {
		f := Frame{Sequence: uint64(i + 1), Width: 640, Height: 480, X: 13, Y: 17, Display: i - 1, Codec: byte(i % 3), Keyframe: i%2 == 0, JPEG: make([]byte, size)}
		if i%2 != 0 {
			f.ViewID = uint64(400 + i)
		}
		for j := range f.JPEG {
			f.JPEG[j] = byte(j*31 + j/chunkSize + i)
		}
		want := append([]byte(nil), f.JPEG...)
		before := time.Now().UnixNano()
		if err := p.SendFrameChecked(f); err != nil {
			t.Fatal(err)
		}
		after := time.Now().UnixNano()
		// Neither later fragments nor caller reuse after Send may change queued data.
		clear(f.JPEG)
		var assembler frameAssembler
		total := (size + chunkSize - 1) / chunkSize
		for index := 0; index < total; index++ {
			select {
			case packet := <-packets:
				if binary.BigEndian.Uint32(packet[16:20]) != uint32(index) || binary.BigEndian.Uint32(packet[20:24]) != uint32(total) {
					t.Fatal("fragment order or count changed")
				}
				stamp := int64(binary.BigEndian.Uint64(packet[24:32]))
				if stamp < before || stamp > after {
					t.Fatal("fragment timestamp was not generated during send")
				}
				out, complete := assembler.add(packet)
				if complete != (index == total-1) {
					t.Fatal("unexpected frame completion")
				}
				if complete && (out.Sequence != f.Sequence || out.Width != f.Width || out.Height != f.Height || out.X != f.X || out.Y != f.Y || out.ViewID != f.ViewID || out.Display != f.Display || out.Codec != f.Codec || out.Keyframe != f.Keyframe || !bytes.Equal(out.JPEG, want)) {
					t.Fatal("frame metadata or payload corrupted")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("frame receive timeout")
			}
		}
	}
}

func BenchmarkSendFrameWebRTC(b *testing.B) {
	for _, size := range []int{4096, 256 * 1024, 1024 * 1024} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			delivered := make(chan struct{}, 1)
			p := frameSendTestPeer(b, func(data []byte) {
				if binary.BigEndian.Uint32(data[16:20])+1 == binary.BigEndian.Uint32(data[20:24]) {
					delivered <- struct{}{}
				}
			})
			f := Frame{Width: 640, Height: 480, JPEG: make([]byte, size)}
			b.SetBytes(int64(size))
			b.ReportAllocs()
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				f.Sequence = uint64(n + 1)
				if err := p.SendFrameChecked(f); err != nil {
					b.Fatal(err)
				}
				select {
				case <-delivered:
				case <-time.After(5 * time.Second):
					b.Fatal("frame receive timeout")
				}
			}
			b.StopTimer()
		})
	}
}
