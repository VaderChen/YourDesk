package clientui

import "testing"

func TestFilesCloseUnreadyPageCanClose(t *testing.T) {
	requested, terminated := 0, 0
	gate := filesCloseGate{request: func() { requested++ }, terminate: func() { terminated++ }}
	gate.requestClose()
	gate.requestClose()
	gate.markReady()
	gate.requestClose()
	gate.forceClose()
	if requested != 0 || terminated != 1 {
		t.Fatalf("unready close requested=%d terminated=%d", requested, terminated)
	}
}

func TestFilesCloseReadyPageWaitsForCleanup(t *testing.T) {
	requested, terminated := 0, 0
	gate := filesCloseGate{request: func() { requested++ }, terminate: func() { terminated++ }}
	gate.markReady()
	gate.requestClose()
	gate.requestClose()
	if requested != 2 || terminated != 0 {
		t.Fatalf("OS gestures must request cleanup, got requested=%d terminated=%d", requested, terminated)
	}
	gate.forceClose()
	gate.forceClose()
	gate.requestClose()
	if requested != 2 || terminated != 1 {
		t.Fatalf("finished cleanup must close once, got requested=%d terminated=%d", requested, terminated)
	}
}

func TestFilesCloseParentExitBypassesPage(t *testing.T) {
	for _, ready := range []bool{false, true} {
		t.Run(map[bool]string{false: "unready", true: "ready"}[ready], func(t *testing.T) {
			terminated := 0
			gate := filesCloseGate{request: func() { t.Fatal("forced close must not wait for JS") }, terminate: func() { terminated++ }}
			if ready {
				gate.markReady()
			}
			gate.forceClose()
			gate.requestClose()
			if terminated != 1 {
				t.Fatalf("forced close count=%d", terminated)
			}
		})
	}
}
