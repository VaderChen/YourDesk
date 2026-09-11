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
