package main

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
	"yourdesk/internal/desktop"
	"yourdesk/internal/p2p"
	"yourdesk/internal/streampipeline"
)

func TestJPEGContinuityRequiresFullFrameAfterGap(t *testing.T) {
	r := newFrameRecovery()
	var s jpegContinuity
	f := p2p.Frame{Sequence: 1, Width: 640, Height: 480, Display: 0}
	if allow, _ := s.accept(f, false); allow {
		t.Fatal("accepted a patch without a base")
	}
	f.Sequence, f.Keyframe = 2, true
	if allow, _ := s.accept(f, false); !allow {
		t.Fatal("rejected full frame")
	}
	s.valid = r.complete(r.epoch())
	f.Sequence, f.Keyframe = 3, false
	if allow, gap := s.accept(f, false); !allow || gap {
		t.Fatal("rejected consecutive patch")
	}
	f.Sequence = 5
	if allow, gap := s.accept(f, false); allow || !gap {
		t.Fatal("accepted patch after missing sequence")
	}
	f.Sequence = 6
	if allow, _ := s.accept(f, false); allow {
		t.Fatal("sequence resumption cannot repair the missing base")
	}
	f.Sequence, f.Keyframe = 7, true
	if allow, _ := s.accept(f, false); !allow {
		t.Fatal("rejected recovery keyframe")
	}
	s.valid = true
	f.Sequence, f.Keyframe, f.ViewID = 8, false, 1
	if allow, _ := s.accept(f, false); allow {
		t.Fatal("accepted delta for another view")
	}
}

func TestFrameRecoveryOldKeyframeCannotClearNewLoss(t *testing.T) {
	r := newFrameRecovery()
	queuedEpoch := r.epoch()
	r.dropped()
	if r.complete(queuedEpoch) || !r.pending() {
		t.Fatal("old queued keyframe hid a later loss")
	}
	if !r.complete(r.epoch()) || r.pending() {
		t.Fatal("fresh full frame did not clear recovery")
	}
}

func TestFrameRecoveryCoalescesRetriesAndStops(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		var calls atomic.Int32
		r := newFrameRecovery()
		for i := 0; i < 100_000; i++ {
			r.dropped()
		}
		if len(r.wake) != 1 {
			t.Fatal("unbounded recovery notifications")
		}
		go r.run(ctx, func(context.Context) { calls.Add(1) })
		synctest.Wait()
		if calls.Load() != 1 {
			t.Fatalf("requests=%d", calls.Load())
		}
		// Returning success is only an ack; no full frame arrived, so retry.
		time.Sleep(6 * time.Second)
		synctest.Wait()
		if calls.Load() != 4 {
			t.Fatalf("missing bounded retry: requests=%d", calls.Load())
		}
		r.complete(r.epoch())
		time.Sleep(6 * time.Second)
		synctest.Wait()
		if calls.Load() != 4 {
			t.Fatal("continued requesting after the frame was repaired")
		}
		cancel()
		synctest.Wait()
	})
}

// Real delta encoder, bounded handoff, recovery state and compositor. Drop the
// final patch, send no later desktop change, then repair solely via recovery.
func TestJPEGBackpressureRepairsLastPatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	bounds := image.Rect(0, 0, 512, 512)
	source := image.NewRGBA(bounds)
	draw.Draw(source, bounds, image.NewUniform(color.RGBA{0, 0, 0, 255}), image.Point{}, draw.Src)
	enc := desktop.DeltaEncoder{TileSize: 64, Quality: 100}
	initial, err := enc.Encode(source, true)
	if err != nil || len(initial) != 1 {
		t.Fatalf("initial: %v", err)
	}
	r := newFrameRecovery()
	var state jpegContinuity
	var visible *image.RGBA
	process := func(f receivedFrame) {
		allow, _ := state.accept(f.Frame, r.pending())
		if !allow {
			r.request()
			return
		}
		decoded, e := jpeg.Decode(bytes.NewReader(f.JPEG))
		if e != nil {
			t.Error(e)
			return
		}
		visible, _ = compositeDecodedFrame(visible, decoded, bounds, int(f.X), int(f.Y), f.Keyframe)
		if f.Keyframe {
			state.valid = r.complete(f.recoveryEpoch)
		}
	}
	frame := func(seq uint64, patch desktop.Patch) p2p.Frame {
		return p2p.Frame{Sequence: seq, Width: 512, Height: 512, X: uint32(patch.X), Y: uint32(patch.Y), Keyframe: patch.Keyframe, JPEG: patch.JPEG}
	}
	process(receivedFrame{Frame: frame(1, initial[0]), recoveryEpoch: r.epoch()})
	for _, y := range []int{0, 128, 256} {
		draw.Draw(source, image.Rect(0, y, 64, y+64), image.NewUniform(color.RGBA{255, 0, 0, 255}), image.Point{}, draw.Src)
	}
	patches, err := enc.Encode(source, false)
	if err != nil || len(patches) != 3 {
		t.Fatalf("patches=%d: %v", len(patches), err)
	}
	release := make(chan struct{})
	processed := make(chan struct{}, 3)
	consumer := streampipeline.NewConsumer(ctx, func(f receivedFrame) {
		select {
		case <-release:
		case <-ctx.Done():
			return
		}
		process(f)
		processed <- struct{}{}
	})
	defer consumer.Close()
	for i, patch := range patches {
		if accepted := r.submit(frame(uint64(i+2), patch), consumer.TrySubmit); accepted != (i < 2) {
			t.Fatalf("unexpected handoff result for patch %d", i)
		}
	}
	close(release)
	for i := 0; i < 2; i++ {
		select {
		case <-processed:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	unchanged, err := enc.Encode(source, false)
	if err != nil || len(unchanged) != 0 || !r.pending() {
		t.Fatal("expected a dropped last patch with no further desktop delta")
	}
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		r.run(ctx, func(context.Context) {
			full, e := enc.Encode(source, true)
			if e != nil {
				t.Error(e)
				return
			}
			r.submit(frame(5, full[0]), consumer.TrySubmit)
		})
	}()
	select {
	case <-processed:
	case <-ctx.Done():
		t.Fatal("last patch recovery timed out")
	}
	cancel()
	<-workerDone
	consumer.Close()
	if r.pending() || !state.valid {
		t.Fatal("fresh keyframe did not restore the base")
	}
	for _, y := range []int{32, 160, 288} {
		red, _, _, _ := visible.At(32, y).RGBA()
		if red < 200*257 {
			t.Fatalf("lost JPEG patch still missing: y=%d red=%d", y, red>>8)
		}
	}
}

func TestFrameRecoveryHighPressureDoesNotWaitForRequester(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := newFrameRecovery()
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		r.run(ctx, func(context.Context) { close(entered); <-release })
	}()
	r.dropped()
	<-entered
	for i := 0; i < 100_000; i++ {
		if r.submit(p2p.Frame{}, func(receivedFrame) bool { return false }) {
			t.Fatal("accepted saturated handoff")
		}
	}
	cancel()
	close(release)
	<-done
}
