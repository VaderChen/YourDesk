package p2p

import "testing"

func TestControlPressureRecovery(t *testing.T) {
	active := false
	for _, step := range []struct {
		buffered uint64
		want     bool
	}{
		{0, false}, {4095, false}, {4096, true}, {4095, true}, {1025, true}, {1024, false}, {2048, false}, {8192, true}, {0, false},
	} {
		active = controlPressure(active, step.buffered)
		if active != step.want {
			t.Fatalf("buffered=%d: active=%v want=%v", step.buffered, active, step.want)
		}
	}
}
