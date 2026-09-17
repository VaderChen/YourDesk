package idleclose

import (
	"testing"
	"time"
)

func TestDisabledAndEnableStartsFresh(t *testing.T) {
	var m Monitor
	now := time.Now()
	if m.Expired(now, false, true) || m.Expired(now.Add(time.Hour), false, true) {
		t.Fatal("default must be off")
	}
	now = now.Add(time.Hour)
	if m.Expired(now, true, true) || m.Expired(now.Add(Timeout-time.Second), true, true) {
		t.Fatal("expired early")
	}
	if !m.Expired(now.Add(Timeout), true, true) {
		t.Fatal("did not expire at fifteen minutes")
	}
	m.Expired(now.Add(Timeout), false, true)
	if m.Expired(now.Add(2*Timeout), true, true) {
		t.Fatal("reenabling reused old idle time")
	}
}

func TestHeartbeatAndRepeatedPointerDoNotReset(t *testing.T) {
	var m Monitor
	now := time.Now()
	m.Expired(now, true, true)
	for i := 0; i <= 900; i++ {
		at := now.Add(time.Duration(i) * time.Second)
		m.Input(at, "move", 0.5, 0.5, false)
		m.Input(at, "command-response", 0, 0, false)
		m.Input(at, "video-status", 0, 0, false)
	}
	if !m.Expired(now.Add(Timeout), true, true) {
		t.Fatal("background messages prevented idle timeout")
	}
}

func TestInputResetsIdle(t *testing.T) {
	for _, kind := range []string{"move", "key", "raw-key", "button", "wheel"} {
		t.Run(kind, func(t *testing.T) {
			var m Monitor
			now := time.Now()
			m.Expired(now, true, true)
			m.Input(now, "move", 10, 10, false)
			at := now.Add(14 * time.Minute)
			m.Input(at, kind, 20, 20, true)
			if m.Expired(now.Add(Timeout), true, true) {
				t.Fatal("input did not reset timer")
			}
			if !m.Expired(at.Add(Timeout), true, true) {
				t.Fatal("new idle interval did not expire")
			}
		})
	}
}

func TestReleaseAndDisconnect(t *testing.T) {
	var m Monitor
	now := time.Now()
	m.Expired(now, true, true)
	m.Input(now.Add(14*time.Minute), "key", 0, 0, false)
	if !m.Expired(now.Add(Timeout), true, true) {
		t.Fatal("synthetic key release reset timer")
	}
	m.Expired(now.Add(Timeout), true, false)
	if m.Expired(now.Add(time.Hour), true, true) {
		t.Fatal("reconnection did not start fresh")
	}
}

func TestPointerJitterAndSlowMovement(t *testing.T) {
	var m Monitor
	now := time.Now()
	m.Expired(now, true, true)
	m.Input(now, "move", 100, 100, false)
	for i := 1; i <= 900; i++ {
		m.Input(now.Add(time.Duration(i)*time.Second), "move", 100+float64(i%3)-1, 100+float64(i%5)-2, false)
	}
	if !m.Expired(now.Add(Timeout), true, true) {
		t.Fatal("jitter kept session alive")
	}
	// 以有效移動錨點累計，不能每個微小位移都重新設定錨點。
	for i := 1; i <= 5; i++ {
		m.Input(now.Add(Timeout), "move", 100+float64(i), 100, false)
	}
	if m.Expired(now.Add(Timeout+time.Second), true, true) {
		t.Fatal("slow real movement ignored")
	}
}
