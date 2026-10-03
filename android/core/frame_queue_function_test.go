package core

import (
	"fmt"
	"testing"
	"yourdeskandroid/core/internal/p2p"
)

func TestFrameRingWrapGrowthAndRelease(t *testing.T) {
	v := NewViewer()
	next, received := uint64(1), uint64(1)
	for round := 0; round < 100; round++ {
		// Varying bursts force wraparound, then growth while the head is nonzero.
		for n := 0; n < 1+round%31; n++ {
			v.enqueueFrame(p2p.Frame{Sequence: next, Keyframe: next == 1, JPEG: []byte{byte(next)}})
			next++
		}
		for n := 0; n < 1+round%29 && received < next; n++ {
			f := v.popFrame()
			if f == nil || f.Sequence != received || f.JPEG[0] != byte(received) {
				t.Fatalf("lost/reordered frame %d: %v", received, f)
			}
			received++
		}
		if len(v.frames) > maxQueuedFrames || v.frameCount != int(next-received) || v.frameBytes != int(next-received) {
			t.Fatal("ring storage or accounting exceeded the logical queue")
		}
	}
	for received < next {
		if f := v.popFrame(); f == nil || f.Sequence != received {
			t.Fatalf("drain reordered frame %d", received)
		}
		received++
	}
	for _, f := range v.frames {
		if f != nil {
			t.Fatal("empty ring retained a consumed payload")
		}
	}
	v.Close()
	if v.frames != nil || v.frameCount != 0 || v.frameHead != 0 {
		t.Fatal("close retained the ring storage")
	}
}

func BenchmarkFrameQueue(b *testing.B) {
	for _, size := range []int{1, 64, 512} {
		b.Run(fmt.Sprintf("Burst%d", size), func(b *testing.B) {
			v := NewViewer()
			f := p2p.Frame{Keyframe: true, JPEG: []byte{1}}
			b.ReportAllocs()
			for b.Loop() {
				for n := 0; n < size; n++ {
					v.enqueueFrame(f)
				}
				for n := 0; n < size; n++ {
					if v.popFrame() == nil {
						b.Fatal("missing frame")
					}
				}
			}
		})
	}
}
