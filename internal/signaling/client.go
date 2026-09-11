package signaling

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"yourdesk/internal/authlog"
	"yourdesk/internal/security"
)

var ErrHostOccupied = errors.New("此裝置已有另一個 Host 連線")

type Client struct {
	binding string
	conn    *websocket.Conn
	room    string
	role    Role
	secret  []byte
	// 驗證失敗時由呼叫端取得密碼，保留同一份 offer 與 WebSocket。
	ResolveSecret func(context.Context) ([]byte, error)
}

func Dial(ctx context.Context, url, room string, role Role, secret []byte) (*Client, error) {
	secureURL, err := security.SecureSignalURL(url)
	if err != nil {
		return nil, err
	}
	client, err := security.TLSHTTPClient(0)
	if err != nil {
		return nil, err
	}
	transport := client.Transport.(*http.Transport)
	transport.ForceAttemptHTTP2 = false
	transport.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
	conn, resp, err := websocket.Dial(ctx, secureURL, &websocket.DialOptions{HTTPClient: client, HTTPHeader: http.Header{"Origin": []string{"https://yourdesk.local"}}})
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("連接 signaling (%s): %w", resp.Status, err)
		}
		return nil, fmt.Errorf("連接 signaling: %w", err)
	}
	authlog.Event("signaling-connected", map[string]any{"role": role})
	c := &Client{conn: conn, room: room, role: role, secret: secret}
	join, err := NewEnvelope(room, role, KindJoin, nil, secret)
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, "join failed")
		return nil, err
	}
	if err := c.write(ctx, join); err != nil {
		_ = conn.Close(websocket.StatusInternalError, "join failed")
		return nil, err
	}
	return c, nil
}

func (c *Client) Send(ctx context.Context, kind Kind, payload any) error {
	e, err := NewEnvelope(c.room, c.role, kind, payload, c.secret)
	if err != nil {
		return err
	}
	if c.binding != "" {
		e.Binding = c.binding
		b, err := e.signingBytes()
		if err != nil {
			return err
		}
		e.Signature = security.Sign(c.secret, b)
	}
	return c.write(ctx, e)
}

func (c *Client) Receive(ctx context.Context) (Envelope, error) {
	for {
		e, err := c.receiveOne(ctx)
		// 中央服務可能先送出待機／休眠前的快取；只略過過去的 offer。
		// 未來時間、answer、直連及驗證錯誤仍照常回報，不放寬驗證。
		if !c.IsDirect() && c.role == RoleViewer && errors.Is(err, ErrMessageTime) && e.Kind == KindOffer && e.Role == RoleHost && time.Now().Unix()-e.Timestamp > 300 {
			continue
		}
		return e, err
	}
}
func (c *Client) receiveOne(ctx context.Context) (Envelope, error) {
	readCtx := ctx
	cancel := func() {}
	if c.IsDirect() {
		readCtx, cancel = context.WithTimeout(ctx, 2*time.Minute)
	}
	_, b, err := c.conn.Read(readCtx)
	cancel()
	if err != nil {
		authlog.Event("signaling-read-failed", map[string]any{"class": authlog.ErrorClass(err), "closeStatus": int(websocket.CloseStatus(err))})
		var closeErr websocket.CloseError
		if c.role == RoleHost && errors.As(err, &closeErr) && closeErr.Code == websocket.StatusPolicyViolation && closeErr.Reason == "role already occupied" {
			return Envelope{}, ErrHostOccupied
		}
		return Envelope{}, err
	}
	var e Envelope
	if err := json.Unmarshal(b, &e); err != nil {
		return Envelope{}, fmt.Errorf("解析 signaling envelope: %w", err)
	}
	authlog.Event("envelope-header", map[string]any{"kind": e.Kind, "role": e.Role, "roomMatches": e.Room == c.room, "bindingMatches": e.Binding == c.binding})
	if e.Room != c.room || e.Role == c.role {
		return Envelope{}, fmt.Errorf("收到錯誤 room 或 role 的 signaling 訊息")
	}
	if c.binding != e.Binding {
		return Envelope{}, errors.New("直連 TLS 工作階段驗證失敗")
	}
	for {
		if err := ctx.Err(); err != nil {
			return Envelope{}, err
		}
		err := e.Verify(c.secret)
		authlog.Event("envelope-received", map[string]any{"kind": e.Kind, "role": e.Role, "ageSeconds": time.Now().Unix() - e.Timestamp, "valid": err == nil, "authFailure": errors.Is(err, ErrAuthentication), "timeFailure": errors.Is(err, ErrMessageTime)})
		if err == nil {
			return e, nil
		}
		if !errors.Is(err, ErrAuthentication) || c.ResolveSecret == nil {
			return e, err
		}
		secret, err := c.ResolveSecret(ctx)
		if err != nil {
			return Envelope{}, err
		}
		c.secret = secret
	}
}

func (c *Client) Close() error {
	return c.conn.Close(websocket.StatusNormalClosure, "done")
}

func (c *Client) write(ctx context.Context, e Envelope) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if authlog.Enabled == "1" {
		var decoded Envelope
		decodeErr := json.Unmarshal(b, &decoded)
		authlog.Event("envelope-send-check", map[string]any{"kind": e.Kind, "role": e.Role, "selfValid": e.Verify(c.secret) == nil, "wireValid": decodeErr == nil && decoded.Verify(c.secret) == nil, "payloadBytes": len(e.Payload)})
	}
	err = c.conn.Write(ctx, websocket.MessageText, b)
	authlog.Event("envelope-sent", map[string]any{"kind": e.Kind, "success": err == nil})
	return err
}

// 待機 offer 每分鐘重新簽署，讓中央服務保存的握手資料保持有效。
// SDP 與 PeerConnection 維持同一份，避免更新時破壞正在進行的握手。
func (c *Client) OfferAndReceive(ctx context.Context, payload any) (Envelope, error) {
	if err := c.Send(ctx, KindOffer, payload); err != nil {
		return Envelope{}, err
	}
	if c.IsDirect() {
		return c.Receive(ctx)
	}
	waiting, cancel := context.WithCancel(ctx)
	stopped := make(chan struct{})
	writeErrors := make(chan error, 1)
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-waiting.Done():
				return
			case <-ticker.C:
				writeCtx, end := context.WithTimeout(waiting, 10*time.Second)
				err := c.Send(writeCtx, KindOffer, payload)
				end()
				if err != nil {
					writeErrors <- err
					cancel()
					return
				}
			}
		}
	}()
	e, err := c.Receive(waiting)
	cancel()
	<-stopped
	if err != nil {
		select {
		case writeErr := <-writeErrors:
			return Envelope{}, writeErr
		default:
		}
	}
	return e, err
}
