package p2p

import (
	"bytes"
	"testing"
)

func framePacket(seq uint64, index, total uint32, data []byte) []byte {
	b := make([]byte, frameHeaderSize+len(data))
	putHeader(b, seq, 640, 360, index, total)
	copy(b[frameHeaderSize:], data)
	return b
}

func TestFrameAssemblerMemoryOwnership(t *testing.T) {
	var a frameAssembler
	packet := framePacket(0, 0, 1, []byte("first"))
	first, ok := a.add(packet)
	if !ok {
		t.Fatal("sequence zero should be valid")
	}
	clear(packet)
	if _, ok := a.add(framePacket(0, 0, 1, []byte("duplicate"))); ok {
		t.Fatal("completed frame delivered twice")
	}
	// Shrinking/growing the fragment table, reverse order, empty/duplicate fragments.
	for seq, total := range []uint32{10, 2, 1, 20, 3} {
		var got Frame
		for i := int(total) - 1; i >= 0; i-- {
			data := []byte{byte(i)}
			if i == 1 {
				data = nil
			}
			p := framePacket(uint64(seq+1), uint32(i), total, data)
			f, complete := a.add(p)
			if complete != (i == 0) {
				t.Fatal("wrong completion state")
			}
			if complete {
				got = f
			}
			if _, duplicate := a.add(p); duplicate {
				t.Fatal("duplicate changed completion state")
			}
			clear(p)
		}
		var want []byte
		for i := uint32(0); i < total; i++ {
			if i != 1 {
				want = append(want, byte(i))
			}
		}
		if !bytes.Equal(got.JPEG, want) || string(first.JPEG) != "first" {
			t.Fatal("output aliased assembler or input storage")
		}
		for _, c := range a.chunks {
			if c != nil {
				t.Fatal("completed frame retains fragments")
			}
		}
	}
}

func TestFrameAssemblerRejectOversizedAndStale(t *testing.T) {
	var a frameAssembler
	if _, ok := a.add(framePacket(100, 0, 1, make([]byte, chunkSize+1))); ok {
		t.Fatal("oversized fragment accepted")
	}
	if _, ok := a.add(framePacket(2, 0, 1, []byte("ok"))); !ok {
		t.Fatal("invalid packet advanced sequence")
	}
	if _, ok := a.add(framePacket(1, 0, 1, []byte("old"))); ok {
		t.Fatal("stale packet accepted")
	}
}

func TestFrameAssemblerDuplicateDoesNotAllocate(t *testing.T) {
	var a frameAssembler
	p := framePacket(1, 1, 3, []byte("payload"))
	a.add(p)
	if n := testing.AllocsPerRun(1000, func() { a.add(p) }); n != 0 {
		t.Fatalf("duplicate allocated: %g", n)
	}
}

func TestFrameAssemblerWorstCaseFragments(t *testing.T) {
	var a frameAssembler
	const total = 1200
	// Maximum fragment count, reverse order, maximum-size payloads.
	payload := bytes.Repeat([]byte{0x35}, chunkSize)
	for i := total - 1; i >= 0; i-- {
		packet := framePacket(1, uint32(i), total, payload)
		frame, complete := a.add(packet)
		if complete != (i == 0) {
			t.Fatal("incorrect completion at maximum fragment count")
		}
		if complete {
			if len(frame.JPEG) != total*chunkSize {
				t.Fatal("truncated maximum-size frame")
			}
			for _, b := range frame.JPEG {
				if b != 0x35 {
					t.Fatal("corrupted maximum-size frame")
				}
			}
		}
	}
	// Retrying a complete frame must not allocate or redeliver any payload.
	duplicate := framePacket(1, 0, total, payload)
	for i := 0; i < 100_000; i++ {
		if _, ok := a.add(duplicate); ok {
			t.Fatal("redelivered completed frame")
		}
	}
	// Replace an incomplete frame by a smaller one without retaining its chunks.
	a.add(framePacket(2, total-1, total, payload))
	if _, ok := a.add(framePacket(3, 0, 1, []byte("next"))); !ok {
		t.Fatal("new frame blocked by incomplete frame")
	}
	for _, c := range a.chunks[:cap(a.chunks)] {
		if c != nil {
			t.Fatal("abandoned frame retains payload")
		}
	}
}
