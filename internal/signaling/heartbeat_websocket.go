package signaling

import (
	"context"
	"net"

	"github.com/coder/websocket"
)

type heartbeatMessage struct {
	kind websocket.MessageType
	data []byte
	err  error
}
type heartbeatWebSocket struct {
	conn     *websocket.Conn
	cancel   context.CancelFunc
	messages chan heartbeatMessage
}

// 持續處理控制訊框；等待輸入密碼或握手完成後仍能回應 Server 的 Ping。
// 只緩衝有限訊息，封包驗證仍由 Client.Receive 執行。
func newHeartbeatWebSocket(conn *websocket.Conn) *heartbeatWebSocket {
	ctx, cancel := context.WithCancel(context.Background())
	h := &heartbeatWebSocket{conn: conn, cancel: cancel, messages: make(chan heartbeatMessage, 16)}
	go func() {
		defer close(h.messages)
		defer cancel()
		for {
			kind, data, err := conn.Read(ctx)
			select {
			case h.messages <- heartbeatMessage{kind, data, err}:
			case <-ctx.Done():
				return
			default:
				conn.CloseNow()
				return
			}
			if err != nil {
				return
			}
		}
	}()
	return h
}
func (h *heartbeatWebSocket) Read(ctx context.Context) (websocket.MessageType, []byte, error) {
	select {
	case <-ctx.Done():
		return 0, nil, ctx.Err()
	case m, ok := <-h.messages:
		if !ok {
			return 0, nil, net.ErrClosed
		}
		return m.kind, m.data, m.err
	}
}
func (h *heartbeatWebSocket) Write(ctx context.Context, kind websocket.MessageType, b []byte) error {
	return h.conn.Write(ctx, kind, b)
}
func (h *heartbeatWebSocket) Close(code websocket.StatusCode, reason string) error {
	defer h.cancel()
	return h.conn.Close(code, reason)
}
