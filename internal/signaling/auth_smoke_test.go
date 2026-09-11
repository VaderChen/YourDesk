package signaling

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/coder/websocket"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"yourdesk/internal/security"
)

// 本機 TLS WebSocket 測試；只信任 httptest 憑證，不修改正式 TLS 設定。
func authSmokeClient(t *testing.T, envelopes []Envelope, secret []byte) (*Client, <-chan error) {
	t.Helper()
	result := make(chan error, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			result <- err
			return
		}
		defer conn.CloseNow()
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		for _, e := range envelopes {
			b, err := json.Marshal(e)
			if err != nil {
				result <- err
				return
			}
			if err = conn.Write(ctx, websocket.MessageText, b); err != nil {
				result <- err
				return
			}
		}
		_, b, err := conn.Read(ctx)
		if err == nil {
			var answer Envelope
			err = json.Unmarshal(b, &answer)
			if err == nil {
				err = answer.Verify(secret)
			}
		}
		result <- err
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	t.Cleanup(cancel)
	conn, _, err := websocket.Dial(ctx, "wss"+strings.TrimPrefix(server.URL, "https"), &websocket.DialOptions{HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.CloseNow() })
	return &Client{conn: conn, room: "smoke-room", role: RoleViewer, secret: secret}, result
}

func TestAuthenticationSmoke(t *testing.T) {
	for _, password := range []string{"test-123456", "測試密碼甲乙丙", "  test-password  ", "AAECAwQFBgcICQoLDA0ODxAREhMUFRYX"} {
		// Case names deliberately do not expose password contents.
		t.Run("password-format", func(t *testing.T) {
			key, err := security.DecodeSecret(password)
			if err != nil {
				t.Fatal(err)
			}
			offer, err := NewEnvelope("smoke-room", RoleHost, KindOffer, map[string]string{"type": "offer", "sdp": "v=0\r\na=test<&>\r\n"}, key)
			if err != nil {
				t.Fatal(err)
			}
			c, answer := authSmokeClient(t, []Envelope{offer}, key)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			received, err := c.Receive(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if received.Kind != KindOffer {
				t.Fatal("not an offer")
			}
			if err = c.Send(ctx, KindAnswer, map[string]string{"type": "answer", "sdp": "v=0\r\n"}); err != nil {
				t.Fatal(err)
			}
			if err = <-answer; err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestAuthenticationSmokeWrongThenCorrect(t *testing.T) {
	key, _ := security.DecodeSecret("correct-test-password")
	wrong, _ := security.DecodeSecret("incorrect-test-password")
	offer, _ := NewEnvelope("smoke-room", RoleHost, KindOffer, map[string]string{"sdp": "v=0\r\n"}, key)
	c, result := authSmokeClient(t, []Envelope{offer}, key)
	c.secret = wrong
	attempts := 0
	c.ResolveSecret = func(context.Context) ([]byte, error) { attempts++; return key, nil }
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := c.Receive(ctx); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 {
		t.Fatalf("retry count %d", attempts)
	}
	if err := c.Send(ctx, KindAnswer, nil); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}
func TestAuthenticationSmokeExpiredOffer(t *testing.T) {
	key, _ := security.DecodeSecret("correct-test-password")
	old, _ := NewEnvelope("smoke-room", RoleHost, KindOffer, nil, key)
	old.Timestamp -= 600
	current, _ := NewEnvelope("smoke-room", RoleHost, KindOffer, nil, key)
	c, result := authSmokeClient(t, []Envelope{old, current}, key)
	c.ResolveSecret = func(context.Context) ([]byte, error) {
		return nil, errors.New("expired offer must not prompt for password")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := c.Receive(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.Send(ctx, KindAnswer, nil); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func TestAuthenticationSmokeRejectsWrongKeyAndTampering(t *testing.T) {
	for _, tampered := range []bool{false, true} {
		t.Run("reject", func(t *testing.T) {
			key, _ := security.DecodeSecret("correct-test-password")
			offer, _ := NewEnvelope("smoke-room", RoleHost, KindOffer, map[string]string{"sdp": "original"}, key)
			c, _ := authSmokeClient(t, []Envelope{offer}, key)
			if tampered {
				// A correctly signed original offer cannot authorize a changed payload.
				offer.Payload = []byte(`{"sdp":"changed"}`)
				if !errors.Is(offer.Verify(key), ErrAuthentication) {
					t.Fatal("tampered offer accepted")
				}
			} else {
				c.secret, _ = security.DecodeSecret("incorrect-test-password")
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				if _, err := c.Receive(ctx); !errors.Is(err, ErrAuthentication) {
					t.Fatalf("wrong password result: %v", err)
				}
			}
		})
	}
}
