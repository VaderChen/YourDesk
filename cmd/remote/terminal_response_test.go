package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"yourdesk/internal/agentremote"
	"yourdesk/internal/p2p"
)

func TestCommandAgentResponsePreservesFailureClass(t *testing.T) {
	for _, tc := range []struct {
		err                  error
		remoteCode, wantCode string
	}{
		{fmt.Errorf("transport: %w", p2p.ErrCommandInterrupted), "", agentremote.CodeRequestInterrupted},
		{fmt.Errorf("busy: %w", p2p.ErrCommandInterrupted), "busy", agentremote.CodeRequestInterrupted},
		{errors.New("file missing"), "failed", "failed"},
		{errors.New("deadline exceeded"), "failed", "failed"},
		{p2p.ErrCommandUnsupported, "", ""},
	} {
		out := commandAgentResponse("request-id", p2p.CommandResponse{Code: tc.remoteCode}, tc.err)
		data, err := json.Marshal(out)
		if err != nil {
			t.Fatal(err)
		}
		var roundtrip agentremote.Response
		if err := json.Unmarshal(data, &roundtrip); err != nil {
			t.Fatal(err)
		}
		if roundtrip.ID != "request-id" || roundtrip.Code != tc.wantCode || roundtrip.Error == "" || roundtrip.Result != nil {
			t.Fatalf("failure type lost in private response: %+v", roundtrip)
		}
	}
	out := commandAgentResponse("ok", p2p.CommandResponse{Result: json.RawMessage(`{"bytes":4}`)}, nil)
	if out.Error != "" || out.Code != "" || string(out.Result) != `{"bytes":4}` {
		t.Fatal("successful response changed")
	}
}
