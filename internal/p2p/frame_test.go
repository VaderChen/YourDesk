package p2p

import (
	"encoding/binary"
	"testing"
)

func TestFrameAssembler(t *testing.T) {
	a := frameAssembler{}
	f := Frame{Sequence: 3, Width: 100, Height: 50, JPEG: []byte("abcdefghijklmnopqrstuvwxyz")}
	for i := 0; i < 3; i++ {
		start := i * 10
		end := start + 10
		if end > len(f.JPEG) {
			end = len(f.JPEG)
		}
		msg := make([]byte, frameHeaderSize+end-start)
		putHeader(msg, f.Sequence, f.Width, f.Height, uint32(i), 3)
		copy(msg[frameHeaderSize:], f.JPEG[start:end])
		got, ok := a.add(msg)
		if i < 2 && ok {
			t.Fatal("incomplete frame")
		}
		if i == 2 {
			if !ok || string(got.JPEG) != string(f.JPEG) {
				t.Fatal("frame mismatch")
			}
		}
	}
}
func putHeader(b []byte, seq uint64, w, h, idx, total uint32) { /* test helper uses little direct fields via production layout */
	binary.BigEndian.PutUint64(b[0:8], seq)
	binary.BigEndian.PutUint32(b[8:12], w)
	binary.BigEndian.PutUint32(b[12:16], h)
	binary.BigEndian.PutUint32(b[16:20], idx)
	binary.BigEndian.PutUint32(b[20:24], total)
}

func TestFrameViewExtension(t *testing.T) {
	packet := make([]byte, frameHeaderSize+8+3)
	putHeader(packet, 1, 1920, 1080, 0, 1)
	packet[40] = 3
	packet[41] = 3
	binary.BigEndian.PutUint64(packet[44:52], 77)
	copy(packet[52:], []byte("av1"))
	var a frameAssembler
	f, ok := a.add(packet)
	if !ok || f.ViewID != 77 || !f.Keyframe || f.Codec != 3 || string(f.JPEG) != "av1" {
		t.Fatalf("視野標頭未正確還原 %+v", f)
	}
	if _, ok = (&frameAssembler{}).add(packet[:50]); ok {
		t.Fatal("接受截斷標頭")
	}
	packet[40] = 4
	if _, ok = (&frameAssembler{}).add(packet); ok {
		t.Fatal("接受未知標頭旗標")
	}
}
