package signaling

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFilesCapabilityKeepsUnknownThroughOlderServer(t *testing.T) {
	for _, tc := range []struct {
		raw          string
		known, value bool
	}{
		{`{"schema":1,"desktop":true}`, false, false},
		{`{"schema":1,"files":false}`, true, false},
		{`{"schema":1,"files":true}`, true, true},
	} {
		var cap HostCapabilities
		if err := json.Unmarshal([]byte(tc.raw), &cap); err != nil {
			t.Fatal(err)
		}
		if (cap.Files != nil) != tc.known || (cap.Files != nil && *cap.Files != tc.value) {
			t.Fatal("capability changed")
		}
		out, err := json.Marshal(cap)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(out), `"files"`) != tc.known {
			t.Fatal("unknown incorrectly advertised as unsupported")
		}
	}
}
