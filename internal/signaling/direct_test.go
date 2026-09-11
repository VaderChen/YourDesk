package signaling

import (
	"testing"
	"time"
)

func TestDirectAttemptLimiterBackoffAndSuccess(t *testing.T) {
	limiter := &directAttemptLimiter{attempts: make(map[string]directAttempt)}
	now := time.Unix(1000, 0)
	if !limiter.allowed("192.0.2.7", now) {
		t.Fatal("new source should be allowed")
	}
	limiter.failed("192.0.2.7", now)
	if limiter.allowed("192.0.2.7", now.Add(999*time.Millisecond)) {
		t.Fatal("first failed attempt should block for one second")
	}
	if !limiter.allowed("192.0.2.7", now.Add(time.Second)) {
		t.Fatal("source should be allowed after first backoff")
	}
	limiter.succeeded("192.0.2.7")
	if !limiter.allowed("192.0.2.7", now) {
		t.Fatal("successful authentication should clear the failure record")
	}
}

func TestDirectSource(t *testing.T) {
	if got := directSource("[2001:db8::1]:47823"); got != "2001:db8::1" {
		t.Fatalf("IPv6 source = %q", got)
	}
}
