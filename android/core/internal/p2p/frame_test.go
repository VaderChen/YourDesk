package p2p

import (
	"encoding/binary"
	"testing"
)

func testChunk(seq uint64, idx, total uint32, payload []byte) []byte {
	b := make([]byte, frameHeaderSize+len(payload))
	binary.BigEndian.PutUint64(b[0:8], seq)
	binary.BigEndian.PutUint32(b[8:12], 1920)
	binary.BigEndian.PutUint32(b[12:16], 1080)
	binary.BigEndian.PutUint32(b[16:20], idx)
	binary.BigEndian.PutUint32(b[20:24], total)
	b[40] = 1
	copy(b[frameHeaderSize:], payload)
	return b
}

func TestAssemblerTinyFrameExactAllocationAndReplay(t *testing.T) {
	var a frameAssembler
	b := testChunk(0, 0, 1, []byte{42})
	f, ok := a.add(b)
	if !ok || len(f.JPEG) != 1 || cap(f.JPEG) != 1 {
		t.Fatalf("tiny frame amplification: len=%d cap=%d ok=%v", len(f.JPEG), cap(f.JPEG), ok)
	}
	b[frameHeaderSize] = 5
	if f.JPEG[0] != 42 {
		t.Fatal("frame aliases reused network buffer")
	}
	if _, ok = a.add(b); ok {
		t.Fatal("completed sequence replayed")
	}
	if _, ok = a.add(testChunk(1, 0, 1, []byte{2})); !ok {
		t.Fatal("next sequence rejected")
	}
}

func TestAssemblerWorstCaseEmptyAndOversizedFragments(t *testing.T) {
	var a frameAssembler
	for i := uint32(0); i < 1200; i++ {
		if _, ok := a.add(testChunk(1, i, 1200, nil)); ok {
			t.Fatal("empty frame accepted")
		}
	}
	if a.bytes != 0 || len(a.chunks) != 0 {
		t.Fatal("empty fragments allocated an assembly")
	}
	if _, ok := a.add(testChunk(2, 0, 1, make([]byte, chunkSize+1))); ok {
		t.Fatal("oversized chunk accepted")
	}
	data := make([]byte, chunkSize)
	for i := uint32(0); i < 1200; i++ {
		if _, ok := a.add(testChunk(3, i, 1200, data)); ok {
			t.Fatal("frame exceeding 32 MiB accepted")
		}
		if a.bytes > maxAssembledFrameBytes {
			t.Fatal("assembly byte budget exceeded")
		}
	}
	if !a.complete || a.bytes != 0 {
		t.Fatal("oversized sequence did not discard retained fragments")
	}
	for _, chunk := range a.chunks {
		if chunk != nil {
			t.Fatal("failed frame retained payload")
		}
	}
	if _, ok := a.add(testChunk(4, 0, 1, []byte{1})); !ok {
		t.Fatal("oversize frame poisoned following sequence")
	}
}

func TestAssemblerOutOfOrderDuplicateAndMetadataMismatch(t *testing.T) {
	var a frameAssembler
	if _, ok := a.add(testChunk(10, 1, 3, []byte{2})); ok {
		t.Fatal("partial frame completed")
	}
	if _, ok := a.add(testChunk(10, 1, 3, []byte{9})); ok || a.bytes != 1 {
		t.Fatal("duplicate consumed bytes or completed")
	}
	mismatch := testChunk(10, 0, 3, []byte{1})
	mismatch[40] = 0
	if _, ok := a.add(mismatch); ok || a.bytes != 1 {
		t.Fatal("metadata mismatch accepted")
	}
	_, _ = a.add(testChunk(10, 0, 3, []byte{1}))
	f, ok := a.add(testChunk(10, 2, 3, []byte{3}))
	if !ok || string(f.JPEG) != string([]byte{1, 2, 3}) || cap(f.JPEG) != 3 {
		t.Fatalf("invalid variable chunk assembly: %+v", f)
	}
	if _, ok := a.add(testChunk(9, 0, 1, []byte{1})); ok {
		t.Fatal("stale sequence accepted")
	}
}

func TestAssemblerBoundsAndAbandonedFrames(t *testing.T) {
	for _, mutate := range []func([]byte){
		func(b []byte) { binary.BigEndian.PutUint32(b[20:24], 1201) },
		func(b []byte) { binary.BigEndian.PutUint32(b[16:20], 1) },
		func(b []byte) { binary.BigEndian.PutUint32(b[8:12], 8193) },
		func(b []byte) { b[41] = 255 },
		func(b []byte) { b[40] = 255 },
	} {
		var a frameAssembler
		b := testChunk(1, 0, 1, []byte{1})
		mutate(b)
		if _, ok := a.add(b); ok || a.total != 0 {
			t.Fatal("invalid header mutated assembly")
		}
	}
	var a frameAssembler
	for seq := uint64(1); seq < 10000; seq++ {
		_, _ = a.add(testChunk(seq, 0, 1200, []byte{1}))
		if a.bytes != 1 || a.received != 1 {
			t.Fatal("abandoned sequence retained bytes")
		}
	}
}
