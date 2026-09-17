package p2p

import (
	"testing"
	"time"
)

func TestTrafficObservation(t *testing.T) {
	baseline := trafficSnapshot{at: time.Now(), sent: 500, received: 800, messages: 50}
	for _, tc := range []struct {
		messages uint64
		duration time.Duration
		state    string
	}{
		{0, 10 * time.Second, "no-receive"},
		{1, 10 * time.Second, "low-receive"},
		{2, 10 * time.Second, "low-receive"},
		{3, 10 * time.Second, "active"},
		{3, 20 * time.Second, "low-receive"},
	} {
		current := trafficSnapshot{at: baseline.at.Add(tc.duration), sent: 600, received: 900, messages: 50 + tc.messages}
		observation := current.since(baseline)
		if observation["state"] != tc.state || observation["receivedMessages"] != tc.messages || observation["receivedBytes"] != uint64(100) || observation["sentBytes"] != uint64(100) {
			t.Fatalf("unexpected observation: %v", observation)
		}
	}
}
