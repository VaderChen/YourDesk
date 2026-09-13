package signaling

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/coder/websocket"
	"yourdeskandroid/core/internal/security"
)

type httpConnection struct {
	client        *http.Client
	base, token   string
	heartbeatV2   bool
	ack, sequence uint64
	writeMu       sync.Mutex
	readMu        sync.Mutex
	cancel        context.CancelFunc
	lifetime      context.Context
	closeOnce     sync.Once
	closeErr      error
}

func dialHTTPS(ctx context.Context, address, room string, role Role, secret []byte) (*Client, error) {
	base, err := security.SignalHTTPSURL(address)
	if err != nil {
		return nil, err
	}
	client, err := security.TLSHTTPClient(30 * time.Second)
	if err != nil {
		return nil, err
	}
	// 不將工作階段權杖或簽署封包跟隨重新導向傳往其他站台。
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	join, err := NewEnvelope(room, role, KindJoin, joinPayload(ctx, role), secret)
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(join)
	if err != nil {
		return nil, err
	}
	h := &httpConnection{client: client, base: base}
	createCtx, finish := context.WithTimeout(ctx, 10*time.Second)
	defer finish()
	// heartbeat-v2 是傳輸層協商；只有 Server 在 201 回覆相同 header
	// 時才採用較長的 HTTPS 活動期限。未知／舊 Server 保持 V1 行為。
	response, err := h.requestWithHeaders(createCtx, "POST", "/signal/session", b, 0, http.Header{
		"YourDesk-Heartbeat-Protocol": []string{"heartbeat-v2"},
	})
	if err != nil {
		client.CloseIdleConnections()
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != 201 {
		client.CloseIdleConnections()
		return nil, fmt.Errorf("HTTPS 建立工作階段失敗：%s", response.Status)
	}
	h.heartbeatV2 = response.Header.Get("YourDesk-Heartbeat-Protocol") == "heartbeat-v2"
	var session struct {
		Token string `json:"token"`
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&session); err != nil || len(session.Token) != 64 {
		client.CloseIdleConnections()
		return nil, fmt.Errorf("HTTPS 工作階段回應無效")
	}
	h.token = session.Token
	heartbeat, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	h.lifetime = heartbeat
	go func() {
		interval := 15 * time.Second
		if h.heartbeatV2 {
			interval = 60 * time.Second
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeat.Done():
				return
			case <-ticker.C:
				pingCtx, end := context.WithTimeout(heartbeat, 5*time.Second)
				r, err := h.request(pingCtx, "POST", "/signal/heartbeat", nil, 0)
				if err == nil {
					r.Body.Close()
				}
				end()
			}
		}
	}()
	return &Client{conn: h, room: room, role: role, secret: secret}, nil
}

func (h *httpConnection) request(ctx context.Context, method, path string, b []byte, sequence uint64) (*http.Response, error) {
	return h.requestWithHeaders(ctx, method, path, b, sequence, nil)
}

func (h *httpConnection) requestWithHeaders(ctx context.Context, method, path string, b []byte, sequence uint64, extra http.Header) (*http.Response, error) {
	r, err := http.NewRequestWithContext(ctx, method, h.base+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	r.Header.Set("Content-Type", "application/json")
	if h.token != "" {
		r.Header.Set("Authorization", "Bearer "+h.token)
	}
	if sequence != 0 {
		r.Header.Set("X-Signal-Sequence", strconv.FormatUint(sequence, 10))
	}
	for key, values := range extra {
		for _, value := range values {
			r.Header.Add(key, value)
		}
	}
	return h.client.Do(r)
}
func httpSignalError(r *http.Response) error {
	if r.StatusCode == 410 {
		var closed struct {
			Code   websocket.StatusCode `json:"code"`
			Reason string               `json:"reason"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&closed) == nil {
			return websocket.CloseError{Code: closed.Code, Reason: closed.Reason}
		}
	}
	return fmt.Errorf("HTTPS 訊號請求失敗：%s", r.Status)
}
func (h *httpConnection) Read(ctx context.Context) (websocket.MessageType, []byte, error) {
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(h.lifetime, cancel)
	defer func() { stop(); cancel() }()
	h.readMu.Lock()
	defer h.readMu.Unlock()
	for {
		r, err := h.request(ctx, "GET", "/signal/messages?ack="+strconv.FormatUint(h.ack, 10), nil, 0)
		if err != nil {
			return 0, nil, err
		}
		if r.StatusCode == 204 {
			r.Body.Close()
			continue
		}
		if r.StatusCode != 200 {
			err = httpSignalError(r)
			r.Body.Close()
			return 0, nil, err
		}
		var item struct {
			Sequence uint64          `json:"sequence"`
			Message  json.RawMessage `json:"message"`
		}
		err = json.NewDecoder(io.LimitReader(r.Body, 300*1024)).Decode(&item)
		r.Body.Close()
		if err != nil {
			return 0, nil, err
		}
		if item.Sequence != h.ack+1 {
			return 0, nil, fmt.Errorf("HTTPS 訊號序號不連續")
		}
		h.ack = item.Sequence
		return websocket.MessageText, item.Message, nil
	}
}
func (h *httpConnection) Write(ctx context.Context, _ websocket.MessageType, b []byte) error {
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(h.lifetime, cancel)
	defer func() { stop(); cancel() }()
	h.writeMu.Lock()
	defer h.writeMu.Unlock()
	sequence := h.sequence + 1
	// 同一序號可重送，伺服器不會重複入列。
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		var r *http.Response
		r, err = h.request(ctx, "POST", "/signal/messages", b, sequence)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			continue
		}
		if r.StatusCode != 204 {
			err = httpSignalError(r)
			r.Body.Close()
			return err
		}
		r.Body.Close()
		h.sequence = sequence
		return nil
	}
	return err
}

// 關閉時同時取消長輪詢、傳訊與心跳；DELETE 使用獨立短期限回收 Server 名額。
func (h *httpConnection) Close(websocket.StatusCode, string) error {
	h.closeOnce.Do(func() {
		h.cancel()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		r, err := h.request(ctx, "DELETE", "/signal/session", nil, 0)
		if err == nil {
			if r.StatusCode != http.StatusNoContent && r.StatusCode != http.StatusGone && r.StatusCode != http.StatusUnauthorized {
				err = httpSignalError(r)
			}
			r.Body.Close()
		}
		h.client.CloseIdleConnections()
		h.closeErr = err
	})
	return h.closeErr
}
