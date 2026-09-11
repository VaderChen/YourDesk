package clientui

import (
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPresenceCapabilitiesSmoke(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "capabilities", true: "legacy"}[legacy], func(t *testing.T) {
			calls := 0
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var in struct {
					Details bool `json:"details"`
				}
				json.NewDecoder(r.Body).Decode(&in)
				w.Header().Set("Content-Type", "application/json")
				if legacy && in.Details {
					http.Error(w, "unknown field", 400)
					return
				}
				if legacy {
					w.Write([]byte(`{"room":true}`))
				} else {
					w.Write([]byte(`{"room":{"online":true,"capabilities":{"schema":1,"desktop":false,"terminal":true,"clipboard":false}}}`))
				}
			}))
			defer srv.Close()
			cert := filepath.Join(t.TempDir(), "ca.pem")
			if e := os.WriteFile(cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0600); e != nil {
				t.Fatal(e)
			}
			t.Setenv("YOURDESK_TLS_CA", cert)
			s := &server{library: Library{Sites: []Site{{ID: "site", Room: "room", Signal: "wss" + strings.TrimPrefix(srv.URL, "https") + "/ws"}}}}
			rec := httptest.NewRecorder()
			s.servePresence(rec, httptest.NewRequest("GET", "/api/presence", nil))
			var result map[string]struct {
				Online       *bool `json:"online"`
				Capabilities *struct {
					Desktop  bool
					Terminal bool
				} `json:"capabilities"`
			}
			if e := json.Unmarshal(rec.Body.Bytes(), &result); e != nil {
				t.Fatal(e)
			}
			got := result["site"]
			if got.Online == nil || !*got.Online {
				t.Fatalf("在線狀態錯誤: %s", rec.Body.String())
			}
			if legacy {
				if got.Capabilities != nil || calls != 2 {
					t.Fatal("舊版降級錯誤")
				}
			} else {
				if got.Capabilities == nil || got.Capabilities.Desktop || !got.Capabilities.Terminal {
					t.Fatal("能力映射錯誤")
				}
			}
		})
	}
}
