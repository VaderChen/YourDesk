package core

import (
	"encoding/json"
	"testing"
	"yourdeskandroid/core/internal/p2p"
	"yourdeskandroid/core/internal/streamconfig"
)

func TestStreamPreferencesCompleteScaleAndQualitySnapshot(t *testing.T) {
	v := NewViewer()
	if _, err := v.prepareStreamPreferencesLocked("high", 75); err == nil {
		t.Fatal("missing capability silently accepted")
	}
	v.receiveStreamControl(p2p.Control{Type: "stream-capabilities", StreamCapabilities: streamconfig.Advertise("session")}, 0)
	for i, scale := range []int{100, 75, 60, 50} {
		r, err := v.prepareStreamPreferencesLocked("high", scale)
		if err != nil || r.Revision != uint64(i+1) || r.Profile != "high" || r.SessionID != "session" {
			t.Fatalf("invalid request: %+v %v", r, err)
		}
		if scale == 100 {
			if r.Source.Resolution.Mode != "native" {
				t.Fatal("100% was not native")
			}
		} else if r.Source.Resolution.Mode != "scale" || r.Source.Resolution.ScalePercent != scale {
			t.Fatal("scale collapsed to a boolean")
		}
		if r.Source.FPS.Mode != "inherit" || r.Source.Rate.Mode != "inherit" {
			t.Fatal("unexpected FPS or rate change")
		}
	}
	for _, scale := range []int{-1, 0, 24, 101} {
		if _, err := v.prepareStreamPreferencesLocked("high", scale); err == nil {
			t.Fatal("invalid scale accepted")
		}
	}
	if _, err := v.prepareStreamPreferencesLocked("unknown", 75); err == nil {
		t.Fatal("invalid profile accepted")
	}
	if v.streamRevision != 4 {
		t.Fatal("invalid requests advanced revision")
	}
}

func TestStreamResultRequiresCurrentSessionAndRevision(t *testing.T) {
	v := NewViewer()
	v.receiveStreamControl(p2p.Control{Type: "stream-capabilities", StreamCapabilities: streamconfig.Advertise("session")}, 0)
	r, _ := v.prepareStreamPreferencesLocked("standard", 75)
	for _, result := range []streamconfig.Result{{SessionID: "old", Revision: r.Revision}, {SessionID: r.SessionID, Revision: r.Revision + 1}, {SessionID: r.SessionID, Revision: 0}} {
		v.receiveStreamControl(p2p.Control{Type: "stream-config-result", StreamResult: &result}, 0)
		if v.streamResult != nil {
			t.Fatal("wrong result accepted")
		}
	}
	result := streamconfig.Result{SessionID: r.SessionID, Revision: r.Revision, Accepted: false, Error: "host rejected"}
	v.receiveStreamControl(p2p.Control{Type: "stream-config-result", StreamResult: &result}, 0)
	var state struct {
		Pending bool
		Error   string
		Result  *streamconfig.Result
	}
	if err := json.Unmarshal([]byte(v.StreamPreferencesJSON()), &state); err != nil {
		t.Fatal(err)
	}
	if state.Pending || state.Error != "host rejected" || state.Result == nil || state.Result.Accepted {
		t.Fatalf("rejection lost: %+v", state)
	}
	_, _ = v.prepareStreamPreferencesLocked("high", 50)
	if v.streamResult != nil || v.streamError != "" {
		t.Fatal("new request kept previous ack")
	}
	v.Close()
	v.receiveStreamControl(p2p.Control{Type: "stream-capabilities", StreamCapabilities: streamconfig.Advertise("old")}, 0)
	if v.streamCapabilities != nil {
		t.Fatal("stale callback revived closed session")
	}
}

func TestStreamCapabilityRefreshResetsRevisionAndRejectsUnsupportedMode(t *testing.T) {
	v := NewViewer()
	caps := streamconfig.Advertise("one")
	v.receiveStreamControl(p2p.Control{Type: "stream-capabilities", StreamCapabilities: caps}, 0)
	_, _ = v.prepareStreamPreferencesLocked("high", 75)
	caps.SessionID = "two"
	caps.ResolutionModes = []string{"native"}
	v.receiveStreamControl(p2p.Control{Type: "stream-capabilities", StreamCapabilities: caps}, 0)
	if v.streamRevision != 0 || v.streamRequest != nil {
		t.Fatal("session refresh retained revision")
	}
	if _, err := v.prepareStreamPreferencesLocked("high", 75); err == nil {
		t.Fatal("unsupported scale accepted")
	}
	if _, err := v.prepareStreamPreferencesLocked("high", 100); err != nil {
		t.Fatal(err)
	}
}

func TestStreamCapabilitiesFromCurrentHostKeyboardEnvelope(t *testing.T) {
	v := NewViewer()
	v.receiveStreamControl(p2p.Control{Type: "keyboard-capabilities", StreamCapabilities: streamconfig.Advertise("desktop-host")}, 0)
	r, err := v.prepareStreamPreferencesLocked("high", 75)
	if err != nil || r.SessionID != "desktop-host" || r.Source.Resolution.ScalePercent != 75 {
		t.Fatalf("current Host envelope lost capabilities: %+v %v", r, err)
	}
}
