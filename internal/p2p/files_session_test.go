package p2p

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/pion/webrtc/v4"
	"yourdesk/internal/security"
	"yourdesk/internal/signaling"
)

func TestFilesSessionNegotiationValidation(t *testing.T) {
	for _, tc := range []struct {
		name             string
		description      transportDescription
		supported, valid bool
	}{
		{"legacy", transportDescription{}, false, true},
		{"legacy-with-new-host", transportDescription{}, true, true},
		{"files", transportDescription{SessionMode: "files", FilesVersion: 1}, true, true},
		{"unsupported-host", transportDescription{SessionMode: "files", FilesVersion: 1}, false, false},
		{"missing-version", transportDescription{SessionMode: "files"}, true, false},
		{"unknown-version", transportDescription{SessionMode: "files", FilesVersion: 2}, true, false},
		{"unknown-mode", transportDescription{SessionMode: "shell", FilesVersion: 1}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateSessionMode(tc.description, tc.supported); (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
		})
	}
}

func TestFilesSessionModeCoveredBySignature(t *testing.T) {
	secret, err := security.DecodeSecret("local-files-session-test")
	if err != nil {
		t.Fatal(err)
	}
	e, err := signaling.NewEnvelope("test-files", signaling.RoleViewer, signaling.KindAnswer,
		transportDescription{SessionDescription: webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: "v=0\r\n"}, SessionMode: "files", FilesVersion: 1}, secret)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Verify(secret); err != nil {
		t.Fatal(err)
	}
	var changed transportDescription
	if err = e.DecodePayload(&changed); err != nil {
		t.Fatal(err)
	}
	changed.SessionMode = ""
	e.Payload, err = json.Marshal(changed)
	if err != nil {
		t.Fatal(err)
	}
	if e.Verify(secret) == nil {
		t.Fatal("mode downgrade was not authenticated")
	}
}

func TestFilesSessionRejectsMediaSends(t *testing.T) {
	// No PC or OS resources are needed: the mode must reject before touching
	// any data channel, encoder, frame payload, or clipboard send gate.
	p := &Peer{}
	p.filesOnly.Store(true)
	if !p.FilesOnly() || p.ClipboardReady() {
		t.Fatal("unexpected file-only state")
	}
	if err := p.SendFrameChecked(Frame{JPEG: []byte{1}}); err == nil {
		t.Fatal("screen send allowed")
	}
	if err := p.SendFrameLimited(context.Background(), Frame{JPEG: []byte{1}}, 1); err == nil {
		t.Fatal("limited screen send allowed")
	}
	if err := p.SendClipboard(context.Background(), []byte{1}); err == nil {
		t.Fatal("clipboard send allowed")
	}
}
