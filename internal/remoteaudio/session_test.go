package remoteaudio

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type fakeDevice struct {
	mode   int
	closed *atomic.Int32
}

func (d *fakeDevice) hardware() bool { return false }
func (d *fakeDevice) close()         { d.closed.Add(1) }
func (d *fakeDevice) process(b []byte) ([]byte, error) {
	if d.mode == 3 {
		return make([]byte, FrameBytes), nil
	}
	return []byte{1, 2, 3}, nil
}
func TestSourceOffSwitchAndLease(t *testing.T) {
	var opened, closed, sent atomic.Int32
	s := &Source{open: func(mode, codec, rate, pref int) (device, error) {
		opened.Add(1)
		return &fakeDevice{mode, &closed}, nil
	}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); s.Run(ctx, func([]byte) error { sent.Add(1); return nil }) }()
	defer func() { cancel(); <-done }()
	time.Sleep(25 * time.Millisecond)
	if opened.Load() != 0 {
		t.Fatal("預設關閉卻啟動原生装置")
	}
	settings := Settings{Enabled: true, Codec: "aac", Profile: "standard", Generation: 1}
	if _, e := s.Configure(settings); e != nil {
		t.Fatal(e)
	}
	wait := func(condition func() bool) {
		t.Helper()
		deadline := time.Now().Add(time.Second)
		for !condition() {
			if time.Now().After(deadline) {
				t.Fatal("聲音工作者逾時")
			}
			time.Sleep(time.Millisecond)
		}
	}
	wait(func() bool { return sent.Load() > 1 })
	if _, e := s.Configure(Settings{Enabled: false, Codec: "aac", Profile: "standard", Generation: 1}); e == nil {
		t.Fatal("相同世代被修改")
	}
	s.mu.Lock()
	s.lease = time.Now().Add(-time.Second)
	s.mu.Unlock()
	wait(func() bool { return closed.Load() == 2 })
	before := sent.Load()
	time.Sleep(25 * time.Millisecond)
	if sent.Load() != before {
		t.Fatal("失去租約後仍傳送")
	}
	settings.Generation = 2
	if _, e := s.Configure(settings); e != nil {
		t.Fatal(e)
	}
	wait(func() bool { return sent.Load() > before })
	settings.Generation = 3
	settings.Enabled = false
	s.Configure(settings)
	wait(func() bool { return closed.Load() == 4 })
	if _, e := s.Configure(Settings{Generation: 2, Codec: "aac", Profile: "high", Enabled: true}); e == nil {
		t.Fatal("舊世代重新啟動擷取")
	}
}
