package signaling

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"github.com/coder/websocket"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"yourdesk/internal/security"
)

func TestHostBinaryAuthenticationSmoke(t *testing.T) {
	binary := os.Getenv("YOURDESK_HOST_SMOKE_BINARY")
	if binary == "" {
		t.Skip("set YOURDESK_HOST_SMOKE_BINARY to test a real host executable")
	}
	password := "local-binary-smoke-password"
	key, _ := security.DecodeSecret(password)
	result := make(chan error, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"yourdesk.local"}})
		if err != nil {
			result <- err
			return
		}
		defer conn.CloseNow()
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		for _, kind := range []Kind{KindJoin, KindOffer} {
			_, raw, err := conn.Read(ctx)
			if err != nil {
				result <- err
				return
			}
			var e Envelope
			if err = json.Unmarshal(raw, &e); err == nil {
				err = e.Verify(key)
			}
			if err != nil {
				result <- fmt.Errorf("%s verification: %w", kind, err)
				return
			}
			if kind == KindJoin {
				var in struct {
					Capabilities *HostCapabilities `json:"capabilities"`
				}
				if json.Unmarshal(e.Payload, &in) != nil || in.Capabilities == nil || in.Capabilities.Schema != 1 || in.Capabilities.OS == "" {
					result <- fmt.Errorf("Host 未回報本機能力")
					return
				}
			}
			if e.Kind != kind || e.Room != "binary-smoke" || e.Role != RoleHost {
				result <- fmt.Errorf("unexpected envelope metadata")
				return
			}
		}
		result <- nil
		<-ctx.Done()
	}))
	defer server.Close()
	cert := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "-signal", "wss"+strings.TrimPrefix(server.URL, "https"), "-room", "binary-smoke", "-secret-stdin", "-codec", "software-jpeg")
	cmd.Env = append(os.Environ(), "YOURDESK_TLS_CA="+cert)
	data, _ := json.Marshal(map[string]string{"secret": password})
	cmd.Stdin = bytes.NewReader(append(data, '\n'))
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cmd.Process.Kill(); cmd.Wait() }()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("host binary handshake timed out")
	}
	t.Log("real host join and offer signatures verified; no answer sent, no desktop capture")
}
