package desktop

import (
	"errors"
	"image"
	"testing"
)

func TestPendingCaptureNoFrameAllocatesNothing(t *testing.T) {
	noFrame := func() error { return ErrNoNewFrame }
	copyPixels := func(*image.RGBA) error { t.Fatal("copied an absent frame"); return nil }
	release := func() { t.Fatal("released an unacquired frame") }
	allocs := testing.AllocsPerRun(100, func() {
		out, err := capturePendingRGBA(image.Rect(0, 0, 3840, 2160), noFrame, copyPixels, release)
		if out != nil || !errors.Is(err, ErrNoNewFrame) {
			t.Fatalf("unexpected output=%v err=%v", out, err)
		}
	})
	if allocs != 0 {
		t.Fatalf("idle capture allocated %.1f objects", allocs)
	}
}

func TestPendingCaptureOwnershipAndRelease(t *testing.T) {
	var acquired, releases int
	source := byte(11)
	take := func() error { acquired++; return nil }
	copyPixels := func(out *image.RGBA) error {
		if acquired != releases+1 {
			t.Fatal("copy without acquired frame")
		}
		for i := range out.Pix {
			out.Pix[i] = source
		}
		return nil
	}
	release := func() { releases++ }
	a, err := capturePendingRGBA(image.Rect(20, 30, 22, 32), take, copyPixels, release)
	if err != nil || a.Bounds() != image.Rect(0, 0, 2, 2) {
		t.Fatalf("first frame: %v %v", a, err)
	}
	source = 42
	b, err := capturePendingRGBA(image.Rect(0, 0, 2, 2), take, copyPixels, release)
	if err != nil || a.Pix[0] != 11 || b.Pix[0] != 42 || releases != 2 {
		t.Fatal("frame overwritten or acquired frame not released")
	}
	failure := errors.New("copy failed")
	out, err := capturePendingRGBA(image.Rect(0, 0, 2, 2), take, func(*image.RGBA) error { return failure }, release)
	if out != nil || !errors.Is(err, failure) || releases != 3 {
		t.Fatal("copy failure leaked frame")
	}
	out, err = capturePendingRGBA(image.Rect(0, 0, 2, 2), func() error { return failure }, copyPixels, release)
	if out != nil || !errors.Is(err, failure) || releases != 3 {
		t.Fatal("take failure released an unacquired frame")
	}
}
