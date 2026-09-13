package signaling

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/coder/websocket"
)

func TestHeartbeatAcceptance(t *testing.T) {
	for _, transport := range []string{"wss", "https"} {
		timeout := "10"
		interval := 60 * time.Second
		if transport == "https" {
			timeout = "180"
			interval = 15 * time.Second
		}
		for _, protocol := range []string{"", "legacy-v1", "scheduler-v2", heartbeatV2, "future-v3"} {
			for _, timing := range []string{"", "0", "-1", "60", "9999999999999999999999"} {
				headers := http.Header{http.CanonicalHeaderKey(heartbeatProtocolHeader): {protocol}, "Yourdesk-Heartbeat-Interval": {timing}, "Yourdesk-Heartbeat-Timeout": {timeout}}
				got := acceptedHeartbeat(headers, transport)
				want := protocol == heartbeatV2 && timing == "60"
				if (got.Protocol == heartbeatV2) != want {
					t.Fatalf("%s/%s/%s: %+v", transport, protocol, timing, got)
				}
				if !want && got.Interval != interval {
					t.Fatal("回退頻率錯誤")
				}
			}
		}
		headers := http.Header{http.CanonicalHeaderKey(heartbeatProtocolHeader): {heartbeatV2}, "Yourdesk-Heartbeat-Interval": {"60"}}
		if acceptedHeartbeat(headers, transport).Protocol == heartbeatV2 {
			t.Fatal("缺少 timeout 仍接受")
		}
	}
}

func TestDialHeartbeatTLS(t *testing.T) {
	for _, transport := range []string{"wss", "https"} {
		for _, v2 := range []bool{false, true} {
			name := transport + "-v1"
			if v2 {
				name = transport + "-v2"
			}
			t.Run(name, func(t *testing.T) {
				secret := []byte("test-only")
				offer, _ := NewEnvelope("room", RoleHost, KindOffer, map[string]string{"sdp": "smoke"}, secret)
				raw, _ := json.Marshal(offer)
				result := make(chan error, 1)
				ping := make(chan error, 1)
				handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/ws" || r.URL.Path == "/signal/session" && r.Method == "POST" {
						if r.Header.Get(heartbeatProtocolHeader) != heartbeatV2 {
							t.Error("未請求 v2")
						}
						if v2 {
							w.Header().Set(heartbeatProtocolHeader, heartbeatV2)
							w.Header().Set("YourDesk-Heartbeat-Interval", "60")
							timeout := "180"
							if transport == "wss" {
								timeout = "10"
							}
							w.Header().Set("YourDesk-Heartbeat-Timeout", timeout)
						}
					}
					check := func(b []byte) error {
						var e Envelope
						if err := json.Unmarshal(b, &e); err != nil {
							return err
						}
						return e.Verify(secret)
					}
					if r.URL.Path == "/ws" {
						c, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"yourdesk.local"}})
						if err != nil {
							return
						}
						defer c.CloseNow()
						ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
						defer cancel()
						_, join, err := c.Read(ctx)
						if err == nil {
							err = check(join)
						}
						if err != nil {
							result <- err
							return
						}
						if err = c.Write(ctx, websocket.MessageText, raw); err != nil {
							result <- err
							return
						}
						if v2 {
							go func() { ping <- c.Ping(ctx) }()
						}
						_, answer, err := c.Read(ctx)
						if err == nil {
							err = check(answer)
						}
						result <- err
						return
					}
					switch {
					case r.URL.Path == "/signal/session" && r.Method == "POST":
						b, _ := io.ReadAll(r.Body)
						if err := check(b); err != nil {
							t.Error(err)
						}
						w.WriteHeader(201)
						json.NewEncoder(w).Encode(map[string]string{"token": strings.Repeat("a", 64)})
					case r.URL.Path == "/signal/messages" && r.Method == "GET":
						json.NewEncoder(w).Encode(map[string]any{"sequence": 1, "message": json.RawMessage(raw)})
					case r.URL.Path == "/signal/messages" && r.Method == "POST":
						b, _ := io.ReadAll(r.Body)
						result <- check(b)
						w.WriteHeader(204)
					default:
						w.WriteHeader(204)
					}
				})
				srv := httptest.NewTLSServer(handler)
				defer srv.Close()
				ca := filepath.Join(t.TempDir(), "ca.pem")
				os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0600)
				t.Setenv("YOURDESK_TLS_CA", ca)
				ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
				defer cancel()
				address := srv.URL
				if transport == "wss" {
					address = "wss" + strings.TrimPrefix(address, "https") + "/ws"
				}
				c, err := Dial(ctx, address, "room", RoleViewer, secret)
				if err != nil {
					t.Fatal(err)
				}
				defer c.Close()
				if (c.heartbeat.Protocol == heartbeatV2) != v2 {
					t.Fatalf("協商模式: %+v", c.heartbeat)
				}
				if v2 && transport == "wss" {
					select {
					case err := <-ping:
						if err != nil {
							t.Fatal(err)
						}
					case <-ctx.Done():
						t.Fatal("未呼叫 Receive 時漏回 Pong")
					}
				}
				if _, err := c.Receive(ctx); err != nil {
					t.Fatal(err)
				}
				if err := c.Send(ctx, KindAnswer, map[string]string{"sdp": "answer"}); err != nil {
					t.Fatal(err)
				}
				if err := <-result; err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

type heartbeatRoundTrip func(*http.Request) (*http.Response, error)

func (f heartbeatRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestHTTPSHeartbeatTimingAndRejection(t *testing.T) {
	for _, v2 := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			settings := acceptedHeartbeat(nil, "https")
			if v2 {
				settings = heartbeatSettings{heartbeatV2, 60 * time.Second, 180 * time.Second}
			}
			var calls atomic.Int32
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			h := &httpConnection{base: "https://test.invalid", heartbeat: settings, cancel: cancel, client: &http.Client{Transport: heartbeatRoundTrip(func(r *http.Request) (*http.Response, error) {
				n := calls.Add(1)
				status := 204
				if n == 2 {
					status = 500
				}
				if n == 3 {
					status = 410
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
			})}}
			go h.runHeartbeat(ctx)
			synctest.Wait()
			time.Sleep(settings.Interval - time.Second)
			synctest.Wait()
			if calls.Load() != 0 {
				t.Fatal("提早發送")
			}
			time.Sleep(time.Second)
			synctest.Wait()
			if calls.Load() != 1 {
				t.Fatal("未依協商頻率發送")
			}
			time.Sleep(settings.Interval)
			synctest.Wait()
			if ctx.Err() != nil {
				t.Fatal("暫時錯誤就取消")
			}
			time.Sleep(settings.Interval)
			synctest.Wait()
			if ctx.Err() == nil {
				t.Fatal("session 被回收卻未取消")
			}
			time.Sleep(settings.Interval)
			synctest.Wait()
			if calls.Load() != 3 {
				t.Fatal("取消後仍發送")
			}
		})
	}
}
