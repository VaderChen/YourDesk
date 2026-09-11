package signaling

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"yourdesk/internal/security"
)

const DirectPort = "47823"

const (
	directMaxFailures = 6
	directMaxBackoff  = time.Minute
)

type directAttempt struct {
	failures int
	blocked  time.Time
}

// directAttemptLimiter 限制同一來源連續驗證失敗，避免密碼猜測。
// 成功驗證後立即清除該來源的失敗紀錄。
type directAttemptLimiter struct {
	mu       sync.Mutex
	attempts map[string]directAttempt
}

func (l *directAttemptLimiter) allowed(source string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, ok := l.attempts[source]
	return !ok || !now.Before(entry.blocked)
}

func (l *directAttemptLimiter) failed(source string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.attempts) >= 4096 {
		for key, entry := range l.attempts {
			if !now.Before(entry.blocked) {
				delete(l.attempts, key)
			}
		}
		if len(l.attempts) >= 4096 {
			return
		}
	}
	entry := l.attempts[source]
	entry.failures = min(entry.failures+1, directMaxFailures)
	backoff := time.Second << (entry.failures - 1)
	if backoff > directMaxBackoff {
		backoff = directMaxBackoff
	}
	entry.blocked = now.Add(backoff)
	l.attempts[source] = entry
}

func (l *directAttemptLimiter) succeeded(source string) {
	l.mu.Lock()
	delete(l.attempts, source)
	l.mu.Unlock()
}

func directSource(remote string) string {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		return remote
	}
	return host
}

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

// 每個 Client 提供一個直連工作階段；中央服務離線不影響此入口。
func ListenDirect(ctx context.Context, address string, secret []byte, handle func(context.Context, *Client)) error {
	config, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	certificate, err := security.ServerTLSIdentity(filepath.Join(config, "YourDesk", "direct-tls"), []string{"localhost"})
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	defer listener.Close()
	slots := make(chan struct{}, 1)
	limiter := &directAttemptLimiter{attempts: make(map[string]directAttempt)}
	mux := http.NewServeMux()
	mux.HandleFunc("/direct", func(w http.ResponseWriter, r *http.Request) {
		source := directSource(r.RemoteAddr)
		if !limiter.allowed(source, time.Now()) {
			http.Error(w, "Too many failed authentication attempts", http.StatusTooManyRequests)
			return
		}
		if r.TLS == nil {
			http.Error(w, "TLS required", 400)
			return
		}
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			http.Error(w, "Client busy", 409)
			return
		}
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"*"}})
		if err != nil {
			return
		}
		defer conn.CloseNow()
		session, cancel := context.WithCancel(ctx)
		defer cancel()
		go func() { <-session.Done(); conn.CloseNow() }()
		joinCtx, joinCancel := context.WithTimeout(session, 10*time.Second)
		_, raw, err := conn.Read(joinCtx)
		joinCancel()
		var join Envelope
		if err != nil || json.Unmarshal(raw, &join) != nil || join.Kind != KindJoin || join.Role != RoleViewer || join.Room != "direct" {
			limiter.failed(source, time.Now())
			return
		}
		binding, err := bindingFor(r.TLS)
		if err != nil || join.Binding != binding {
			limiter.failed(source, time.Now())
			return
		}
		// 遠端顯示 必須先證明持有連線密碼，才可啟動主機流程。
		// 驗證失敗時不回傳任何由密碼簽署的資料，避免形成離線猜測樣本。
		if err := join.Verify(secret); err != nil {
			limiter.failed(source, time.Now())
			return
		}
		limiter.succeeded(source)
		client := &Client{conn: conn, room: "direct", role: RoleHost, secret: secret, binding: binding}
		handle(session, client)
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate}}}
	finished := make(chan struct{})
	defer close(finished)
	go func() {
		select {
		case <-ctx.Done():
			server.Close()
		case <-finished:
		}
	}()
	err = server.ServeTLS(listener, "", "")
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (c *Client) IsDirect() bool { return c.binding != "" }
