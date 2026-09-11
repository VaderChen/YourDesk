package runtimeenv

import "testing"

func TestDesktopDetection(t *testing.T) {
	for _, c := range []struct {
		os, x, wayland string
		headless       bool
	}{{"linux", "", "", true}, {"linux", ":0", "", false}, {"linux", "", "wayland-0", false}, {"darwin", "", "", false}, {"windows", "", "", false}} {
		if got := headlessSession(c.os, c.x, c.wayland); got != c.headless {
			t.Fatalf("%+v: %v", c, got)
		}
	}
}
