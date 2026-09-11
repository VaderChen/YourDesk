package p2p

import (
	"bytes"
	"context"
	"testing"
	"time"
	"yourdesk/internal/streampipeline"
)

func TestAssemblerDoubleBufferOwnership(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	results := make(chan Frame, 100)
	consumer := streampipeline.NewConsumer(ctx, func(f Frame) { results <- f })
	defer consumer.Close()
	var assembler frameAssembler
	for sequence := uint64(1); sequence <= 100; sequence++ {
		// Reverse fragment order; duplicate one fragment. Frame 50 is incomplete.
		for _, index := range []uint32{2, 1, 1, 0} {
			if sequence == 50 && index == 0 {
				continue
			}
			packet := make([]byte, frameHeaderSize+100)
			putHeader(packet, sequence, 640, 360, index, 3)
			copy(packet[frameHeaderSize:], bytes.Repeat([]byte{byte(sequence)}, 100))
			frame, complete := assembler.add(packet)
			// Reusing network input must not corrupt a queued frame.
			clear(packet)
			if complete && !consumer.Submit(frame) {
				t.Fatal("handoff canceled")
			}
		}
	}
	for sequence := uint64(1); sequence <= 100; sequence++ {
		if sequence == 50 {
			continue
		}
		select {
		case f := <-results:
			if f.Sequence != sequence || !bytes.Equal(f.JPEG, bytes.Repeat([]byte{byte(sequence)}, 300)) {
				t.Fatalf("frame %d: corrupted payload or order, got %d", sequence, f.Sequence)
			}
		case <-ctx.Done():
			t.Fatal("receive timeout")
		}
	}
	select {
	case f := <-results:
		t.Fatalf("unexpected frame %d", f.Sequence)
	default:
	}
}
