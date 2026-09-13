package signaling

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
)

const DirectPort = "47823"

// 僅接受 IP literal，避免把一般站台 ID 或任意 URL 當作直連端點。
func DirectAddress(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if ip, err := netip.ParseAddr(strings.Trim(value, "[]")); err == nil {
		return net.JoinHostPort(ip.String(), DirectPort), true
	}
	host, port, err := net.SplitHostPort(value)
	if err != nil {
		return "", false
	}
	ip, err := netip.ParseAddr(host)
	n, portErr := strconv.Atoi(port)
	if err != nil || portErr != nil || n < 1 || n > 65535 {
		return "", false
	}
	return net.JoinHostPort(ip.String(), port), true
}

func bindingFor(state *tls.ConnectionState) (string, error) {
	key, err := state.ExportKeyingMaterial("EXPORTER-YourDesk-direct-v1", nil, 32)
	return base64.RawURLEncoding.EncodeToString(key), err
}

// 離線無公共 CA；TLS 的對端身分由既有密碼 HMAC 加上 TLS exporter 驗證。
// Receive 在密碼與 exporter 同時驗證成功前，不把 SDP 交給 WebRTC。
func DialDirect(ctx context.Context, address string, secret []byte) (*Client, error) {
	var binding string
	transport := &http.Transport{ForceAttemptHTTP2: false}
	transport.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		dialer := tls.Dialer{NetDialer: &net.Dialer{Timeout: 8 * time.Second}, Config: &tls.Config{
			MinVersion:         tls.VersionTLS13,
			InsecureSkipVerify: true, // CA 驗證以 Receive 中密碼驗證的 TLS exporter 取代，僅限離線直連。
			VerifyConnection: func(state tls.ConnectionState) error {
				if len(state.PeerCertificates) == 0 {
					return errors.New("直連缺少 TLS 憑證")
				}
				cert := state.PeerCertificates[0]
				if time.Now().Before(cert.NotBefore) || time.Now().After(cert.NotAfter) {
					return errors.New("直連 TLS 憑證已失效")
				}
				return cert.CheckSignature(cert.SignatureAlgorithm, cert.RawTBSCertificate, cert.Signature)
			},
		}}
		conn, err := dialer.DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		state := conn.(*tls.Conn).ConnectionState()
		binding, err = bindingFor(&state)
		if err != nil {
			conn.Close()
			return nil, err
		}
		return conn, nil
	}
	httpClient := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("直連禁止重新導向") }}
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(dialCtx, "wss://"+address+"/direct", &websocket.DialOptions{HTTPClient: httpClient})
	if err != nil {
		return nil, fmt.Errorf("IP 直連失敗：%w", err)
	}
	client := &Client{conn: conn, room: "direct", role: RoleViewer, secret: secret, binding: binding}
	if err = client.Send(ctx, KindJoin, nil); err != nil {
		conn.CloseNow()
		return nil, err
	}
	return client, nil
}
