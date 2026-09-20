package core

import (
	"encoding/json"
	"fmt"
	"slices"
	"yourdeskandroid/core/internal/p2p"
	"yourdeskandroid/core/internal/streamconfig"
)

func (v *Viewer) resetStreamLocked() {
	v.streamCapabilities = nil
	v.streamRevision = 0
	v.streamRequest = nil
	v.streamResult = nil
	v.streamError = ""
}

// Installed before the Peer opens its DataChannel, so the initial capability
// message cannot race registration of the Android bridge.
func (v *Viewer) receiveStreamControl(c p2p.Control, generation uint64) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.generation != generation {
		return
	}
	// Current desktop Hosts piggyback this on keyboard-capabilities; accept
	// the dedicated envelope too for forward/backward compatibility.
	if (c.Type == "stream-capabilities" || c.Type == "keyboard-capabilities") && c.StreamCapabilities != nil {
		caps := *c.StreamCapabilities
		if caps.SchemaVersion != streamconfig.Version || caps.SessionID == "" || len(caps.SessionID) > 128 {
			return
		}
		if v.streamCapabilities == nil || v.streamCapabilities.SessionID != caps.SessionID {
			v.resetStreamLocked()
		}
		caps.ResolutionModes = slices.Clone(caps.ResolutionModes)
		caps.FPSModes = slices.Clone(caps.FPSModes)
		caps.RateModes = slices.Clone(caps.RateModes)
		v.streamCapabilities = &caps
	}
	if c.Type == "stream-config-result" && c.StreamResult != nil {
		result := *c.StreamResult
		if v.streamRequest == nil || result.SessionID != v.streamRequest.SessionID || result.Revision != v.streamRequest.Revision {
			return
		}
		v.streamResult = &result
		v.streamError = result.Error
	}
}

func (v *Viewer) prepareStreamPreferencesLocked(profile string, scalePercent int) (streamconfig.Request, error) {
	if v.streamCapabilities == nil {
		return streamconfig.Request{}, fmt.Errorf("遠端尚未公告新版串流設定能力")
	}
	if scalePercent < 25 || scalePercent > 100 {
		return streamconfig.Request{}, fmt.Errorf("縮放比例必須為 25 至 100")
	}
	// No viewport cap is requested; native/scale are relative to the Host's
	// source dimensions, not the Android preview dimensions.
	r := streamconfig.Compose(profile, 8192, 8192, false)
	r.SessionID = v.streamCapabilities.SessionID
	r.Revision = v.streamRevision + 1
	r.Source.Resolution = streamconfig.Resolution{Mode: "native"}
	if scalePercent != 100 {
		r.Source.Resolution = streamconfig.Resolution{Mode: "scale", ScalePercent: scalePercent}
	}
	if !slices.Contains(v.streamCapabilities.ResolutionModes, r.Source.Resolution.Mode) || !slices.Contains(v.streamCapabilities.FPSModes, "inherit") || !slices.Contains(v.streamCapabilities.RateModes, "inherit") {
		return streamconfig.Request{}, fmt.Errorf("遠端不支援要求的解析度／FPS／碼率設定")
	}
	if err := r.Validate(); err != nil {
		return streamconfig.Request{}, err
	}
	v.streamRevision = r.Revision
	v.streamRequest, v.streamResult, v.streamError = &r, nil, ""
	return r, nil
}

// SendStreamPreferences sends one complete, validated preference snapshot.
// A nil error means queued/sent, not applied: StreamPreferencesJSON exposes
// the matching Host acknowledgement and its effective settings separately.
func (v *Viewer) SendStreamPreferences(profile string, scalePercent int) error {
	v.streamSendMu.Lock()
	defer v.streamSendMu.Unlock()
	v.mu.Lock()
	p, generation := v.peer, v.generation
	if p == nil {
		v.mu.Unlock()
		return fmt.Errorf("遠端尚未連線")
	}
	r, err := v.prepareStreamPreferencesLocked(profile, scalePercent)
	v.mu.Unlock()
	if err != nil {
		return err
	}
	err = p.SendControl(p2p.Control{Type: "stream-config", StreamConfig: &r})
	if err != nil {
		v.mu.Lock()
		if v.generation == generation && v.streamRequest != nil && v.streamRequest.Revision == r.Revision {
			v.streamError = err.Error()
		}
		v.mu.Unlock()
	}
	return err
}

func (v *Viewer) StreamPreferencesJSON() string {
	v.mu.Lock()
	state := struct {
		Capabilities *streamconfig.Capabilities `json:"capabilities,omitempty"`
		Request      *streamconfig.Request      `json:"request,omitempty"`
		Result       *streamconfig.Result       `json:"result,omitempty"`
		Pending      bool                       `json:"pending"`
		Error        string                     `json:"error,omitempty"`
	}{v.streamCapabilities, v.streamRequest, v.streamResult, v.streamRequest != nil && v.streamResult == nil && v.streamError == "", v.streamError}
	v.mu.Unlock()
	b, _ := json.Marshal(state)
	return string(b)
}
