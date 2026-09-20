package hostsession

import (
	"errors"
	"image"
	"testing"
	"time"
	"yourdesk/internal/desktop"
)

func TestIdleJPEGRefreshRepairsUnobservableLastPacketLoss(t *testing.T) {
	var r idleFrameRefresh
	now := time.Now()
	snapshots := 0
	want := image.NewRGBA(image.Rect(0, 0, 32, 32))
	noChange := func() (image.Image, error) { return nil, desktop.ErrNoNewFrame }
	snapshot := func() (image.Image, error) { snapshots++; return want, nil }
	for _, elapsed := range []time.Duration{0, time.Second, 4 * time.Second} {
		if _, err := r.capture(now.Add(elapsed), true, false, noChange, snapshot); !errors.Is(err, desktop.ErrNoNewFrame) {
			t.Fatal("refreshed too early")
		}
	}
	if img, err := r.capture(now.Add(5*time.Second), true, false, noChange, snapshot); err != nil || img != want || snapshots != 1 {
		t.Fatal("static JPEG never refreshed after the last packet disappeared")
	}
	if _, err := r.capture(now.Add(6*time.Second), true, false, noChange, snapshot); !errors.Is(err, desktop.ErrNoNewFrame) || snapshots != 1 {
		t.Fatal("static refresh is not rate-limited")
	}
	if _, err := r.capture(now.Add(10*time.Second), true, false, noChange, snapshot); err != nil || snapshots != 2 {
		t.Fatal("periodic refresh did not continue")
	}
}

func TestIdleRefreshExplicitRequestAndVideoMode(t *testing.T) {
	var r idleFrameRefresh
	now := time.Now()
	snapshots := 0
	noChange := func() (image.Image, error) { return nil, desktop.ErrNoNewFrame }
	snapshot := func() (image.Image, error) { snapshots++; return image.NewRGBA(image.Rect(0, 0, 1, 1)), nil }
	for _, at := range []time.Time{now, now.Add(time.Hour)} {
		if _, err := r.capture(at, false, false, noChange, snapshot); !errors.Is(err, desktop.ErrNoNewFrame) {
			t.Fatal("video mode unexpectedly took periodic screenshots")
		}
	}
	if _, err := r.capture(now.Add(time.Hour), false, true, noChange, snapshot); err != nil || snapshots != 1 {
		t.Fatal("explicit recovery did not capture a static desktop")
	}
	failure := errors.New("capture failure")
	if _, err := r.capture(now, true, true, func() (image.Image, error) { return nil, failure }, snapshot); !errors.Is(err, failure) || snapshots != 1 {
		t.Fatal("swallowed a real capture failure")
	}
}
