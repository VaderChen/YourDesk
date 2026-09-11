package clientui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
	"yourdesk/internal/diagnostics"
)

type smokePipe struct{ bytes.Buffer }

func (*smokePipe) Close() error { return nil }
func TestAutoStreamReleaseSmoke(t *testing.T) {
	for _, fps := range []float64{4.9, 15, 29.8} {
		t.Run(fmt.Sprintf("%.1f-FPS", fps), func(t *testing.T) {
			pipe := &smokePipe{}
			child := &process{kind: "viewer", siteID: "smoke", stage: "connected", stdin: pipe}
			s := &server{configPath: filepath.Join(t.TempDir(), "sites.json"), children: map[string]*process{"viewer:smoke": child}, preferences: Preferences{Language: "auto", Theme: "default", SourceFPSLimit: 10, BitrateLimitMbps: 12, KeyframeInterval: 10}}
			call := func(body map[string]any) map[string]any {
				t.Helper()
				b, _ := json.Marshal(body)
				w := httptest.NewRecorder()
				s.autoStream(w, httptest.NewRequest("POST", "/api/stream-auto", bytes.NewReader(b)))
				if w.Code != 200 {
					t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
				}
				var out map[string]any
				if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil {
					t.Fatal(e)
				}
				return out
			}
			call(map[string]any{"mode": "latency", "id": "smoke"})
			if !bytes.Contains(pipe.Bytes(), []byte(`"probeFPS":30`)) {
				t.Fatal("probe not requested")
			}
			d := child.diagnostic
			d.Samples = []diagnostics.Sample{{Seconds: 5, SourceFPS: fps, FPSLimit: 30, Received: 30, Codec: "H.264"}, {Seconds: 5, SourceFPS: fps, FPSLimit: 30, Received: 30, Codec: "H.264"}}
			args := map[string]any{"mode": "latency", "id": "smoke", "startedAt": d.StartedAt}
			result := call(args)
			proposed := result["preferences"].(map[string]any)["sourceFPSLimit"].(float64)
			if proposed < 5 || proposed > 30 {
				t.Fatal("outside 5–30 range")
			}
			if fps > 29 && proposed != 30 {
				t.Fatal("old 10 FPS preference limited probe")
			}
			if s.preferences.SourceFPSLimit != 10 {
				t.Fatal("preview applied settings")
			}
			if _, e := os.Stat(filepath.Join(filepath.Dir(s.configPath), "preferences.json")); !os.IsNotExist(e) {
				t.Fatal("preview wrote preferences")
			}
			if !bytes.Contains(pipe.Bytes(), []byte(`"probeFPS":0`)) {
				t.Fatal("preview did not stop probe")
			}
			// 等待使用者期間超過取樣期限，仍應套用已顯示的建議，保留其他新偏好。
			d.StartedAt = time.Now().Add(-time.Minute).UnixMilli()
			args["startedAt"] = d.StartedAt
			s.preferences.BitrateLimitMbps = 18
			args["apply"] = true
			call(args)
			if s.preferences.SourceFPSLimit != int(proposed) || s.preferences.BitrateLimitMbps != 18 {
				t.Fatal("apply differs from preview or overwrote unrelated setting")
			}
			if child.diagnostic.Proposal != nil {
				t.Fatal("proposal reusable after apply")
			}
			call(map[string]any{"mode": "latency", "id": "smoke"})
			id := child.diagnostic.StartedAt
			old := s.preferences.SourceFPSLimit
			call(map[string]any{"mode": "latency", "id": "smoke", "startedAt": id, "cancel": true})
			if s.preferences.SourceFPSLimit != old || child.diagnostic != nil {
				t.Fatal("cancel changed settings or retained probe")
			}
		})
	}
}
func TestMCPWhitelistReleaseSmoke(t *testing.T) {
	s := &server{preferences: Preferences{MCPWhitelistEnabled: true, MCPWhitelist: []string{"127.0.0.1"}}}
	for _, row := range []struct {
		address        string
		allowed, token bool
	}{{"127.0.0.1:3000", true, false}, {"[::ffff:127.0.0.1]:3000", true, false}, {"192.0.2.1:3000", false, true}, {"bad", false, true}} {
		a, b := s.mcpRemoteAccess(row.address)
		if a != row.allowed || b != row.token {
			t.Fatalf("wrong access for %s", row.address)
		}
	}
	s.preferences.MCPWhitelistEnabled = false
	a, b := s.mcpRemoteAccess("192.0.2.1:3000")
	if !a || !b {
		t.Fatal("disabled whitelist must require token")
	}
}
