package idleclose

import (
	"testing"
	"time"
)

func TestCommittedTextResetsIdleTimeout(t *testing.T) {
	var m Monitor
	start := time.Unix(0, 0)
	m.Expired(start, true, true)
	m.Input(start.Add(Timeout-time.Second), "text", 0, 0, false)
	if m.Expired(start.Add(Timeout), true, true) {
		t.Fatal("IME text did not reset idle timeout")
	}
	if !m.Expired(start.Add(2*Timeout), true, true) {
		t.Fatal("idle timeout never expires")
	}
}
