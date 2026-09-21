package hostsession

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"yourdesk/internal/p2p"
	"yourdesk/internal/peertransport"
	"yourdesk/internal/signaling"
)

const filesSmokeMode peertransport.Mode = "hostsession-files-loopback-test"

var filesSmokeBackendOnce sync.Once
var filesSmokeBackendError error

type filesSmokeLink struct{ net.PacketConn }

func (filesSmokeLink) Offer() string { return "isolated-loopback-fixture" }

type filesSmokeBackend struct{}

func (filesSmokeBackend) Host(context.Context) (peertransport.Link, error) {
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	return filesSmokeLink{conn}, nil
}
func (b filesSmokeBackend) Viewer(ctx context.Context, offer string) (peertransport.Link, error) {
	if offer != "isolated-loopback-fixture" {
		return nil, errors.New("unexpected loopback fixture offer")
	}
	return b.Host(ctx)
}

// A real local TLS/WebSocket relay plus signed signaling clients. UDP is pinned
// to loopback by the virtual transport path, so these tests never use STUN or a
// public signaling server. They do not list/read/write the user's home: Stream
// only registers the file handlers and the test calls session.capabilities.
func filesSmokeSignals(t *testing.T, ctx context.Context) (*signaling.Client, *signaling.Client) {
	t.Helper()
	filesSmokeBackendOnce.Do(func() { filesSmokeBackendError = peertransport.Register(filesSmokeMode, filesSmokeBackend{}) })
	if filesSmokeBackendError != nil {
		t.Fatal(filesSmokeBackendError)
	}
	secret := []byte("isolated-file-session-test-secret")
	const room = "isolated-file-session-room"
	toHost, toViewer := make(chan []byte, 16), make(chan []byte, 16)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"yourdesk.local"}})
		if err != nil {
			return
		}
		defer conn.CloseNow()
		_, raw, err := conn.Read(ctx)
		var join signaling.Envelope
		if err != nil || json.Unmarshal(raw, &join) != nil || join.Verify(secret) != nil || join.Kind != signaling.KindJoin || join.Room != room {
			return
		}
		incoming, outgoing := toHost, toViewer
		if join.Role == signaling.RoleViewer {
			incoming, outgoing = toViewer, toHost
		} else if join.Role != signaling.RoleHost {
			return
		}
		readDone := make(chan struct{})
		go func() {
			defer close(readDone)
			for {
				_, packet, err := conn.Read(ctx)
				if err != nil {
					return
				}
				var envelope signaling.Envelope
				if json.Unmarshal(packet, &envelope) != nil || envelope.Verify(secret) != nil || envelope.Room != room || envelope.Role != join.Role {
					return
				}
				select {
				case outgoing <- packet:
				case <-ctx.Done():
					return
				}
			}
		}()
		for {
			select {
			case packet := <-incoming:
				if conn.Write(ctx, websocket.MessageText, packet) != nil {
					return
				}
			case <-readDone:
				return
			case <-ctx.Done():
				return
			}
		}
	}))
	t.Cleanup(server.Close)
	ca := filepath.Join(t.TempDir(), "fixture-ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YOURDESK_TLS_CA", ca)
	t.Setenv("YOURDESK_AUTH_LOG_DIR", t.TempDir())
	address := "wss" + strings.TrimPrefix(server.URL, "https")
	host, err := signaling.Dial(ctx, address, room, signaling.RoleHost, secret)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = host.Close() })
	viewer, err := signaling.Dial(ctx, address, room, signaling.RoleViewer, secret)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = viewer.Close() })
	return host, viewer
}

func waitFilesSmoke(t *testing.T, ctx context.Context, ready func() bool) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for !ready() {
		select {
		case <-ctx.Done():
			t.Fatal("loopback file session did not become ready:", ctx.Err())
		case <-ticker.C:
		}
	}
}

func TestFilesSessionNegotiatesBeforeDesktopResources(t *testing.T) {
	for _, headless := range []bool{false, true} {
		name := "desktop-capable-host"
		if headless {
			name = "headless-host"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			hostSignal, viewerSignal := filesSmokeSignals(t, ctx)
			finished := make(chan error, 1)
			go func() {
				// The invalid codec is a tripwire: the desktop branch must never
				// proceed to validation, permission prompts or encoder allocation.
				finished <- Stream(ctx, hostSignal, Options{Headless: headless, Transport: filesSmokeMode, Codec: "invalid-desktop-codec-fixture"})
			}()
			viewer, err := p2p.NewFilesViewerWithTransport(ctx, viewerSignal, filesSmokeMode)
			if err != nil {
				t.Fatal(err)
			}
			defer viewer.Close()
			waitFilesSmoke(t, ctx, func() bool { return viewer.SupportsCommand("session.capabilities") })
			host := incomingPeer.Load()
			if host == nil || !host.FilesOnly() || !viewer.FilesOnly() {
				t.Fatal("signed file-only session was not selected at both ends")
			}
			for _, method := range viewer.RemoteCommands().Methods {
				switch method {
				case "capabilities.get", "ping", "session.status", "session.disconnect", "network.probe", "session.capabilities":
				default:
					if !strings.HasPrefix(method, "xfer.") {
						t.Fatalf("file session exposed non-file command %q", method)
					}
				}
			}
			response, err := viewer.CallCommand(ctx, "session.capabilities")
			if err != nil {
				t.Fatal(err)
			}
			var caps struct{ Desktop, Terminal, Clipboard, Files bool }
			if err := json.Unmarshal(response.Result, &caps); err != nil || caps.Desktop || caps.Terminal || caps.Clipboard || !caps.Files {
				t.Fatalf("unexpected file session capabilities: %+v %v", caps, err)
			}
			if host.SendFrameChecked(p2p.Frame{}) == nil || host.SendClipboard(ctx, []byte("fixture")) == nil || host.ClipboardReady() {
				t.Fatal("file-only Host permitted desktop media or clipboard")
			}
			if _, err := viewer.CallCommand(ctx, "terminal.open"); !errors.Is(err, p2p.ErrCommandUnsupported) {
				t.Fatalf("file-only viewer reached terminal command: %v", err)
			}
			_ = viewer.Close()
			cancel()
			select {
			case err := <-finished:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("file Host did not stop")
			}
		})
	}
}

func TestFilesSessionLegacyNegotiationCompatibility(t *testing.T) {
	for _, filesViewer := range []bool{false, true} {
		name := "legacy-viewer-new-host"
		if filesViewer {
			name = "file-viewer-legacy-host-fails-closed"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			hostSignal, viewerSignal := filesSmokeSignals(t, ctx)
			type result struct {
				peer *p2p.Peer
				err  error
			}
			finished := make(chan result, 1)
			go func() {
				host, err := p2p.NewHostWithTransport(ctx, hostSignal, filesSmokeMode, nil, p2p.HostOptions{FilesOnly: !filesViewer})
				finished <- result{host, err}
			}()
			if filesViewer {
				viewer, err := p2p.NewFilesViewerWithTransport(ctx, viewerSignal, filesSmokeMode)
				if err == nil || viewer != nil || !strings.Contains(err.Error(), "獨立檔案傳輸") {
					t.Fatalf("legacy Host was not rejected before an answer: %v", err)
				}
				cancel()
				select {
				case out := <-finished:
					if out.peer != nil {
						_ = out.peer.Close()
						t.Fatal("legacy Host accepted a file viewer as desktop")
					}
				case <-time.After(3 * time.Second):
					t.Fatal("legacy Host handshake did not stop")
				}
				return
			}
			viewer, err := p2p.NewViewerWithTransport(ctx, viewerSignal, filesSmokeMode, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer viewer.Close()
			out := <-finished
			if out.err != nil {
				t.Fatal(out.err)
			}
			defer out.peer.Close()
			waitFilesSmoke(t, ctx, func() bool { return viewer.Connected() && out.peer.Connected() })
			if viewer.FilesOnly() || out.peer.FilesOnly() {
				t.Fatal("legacy desktop viewer was forced into file-only mode")
			}
		})
	}
}
