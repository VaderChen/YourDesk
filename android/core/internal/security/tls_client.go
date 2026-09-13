package security

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	_ "embed"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

//go:embed server-cert.crt
var serverTrust []byte

func SecureSignalURL(value string) (string, error) {
	address, err := url.Parse(value)
	if err != nil {
		return "", err
	}
	if address.Hostname() == "" || address.User != nil || (address.Scheme != "ws" && address.Scheme != "wss" && address.Scheme != "https") {
		return "", errors.New("配對服務必須使用有效的 WSS 或 HTTPS 網址")
	}
	if address.Scheme == "ws" {
		address.Scheme = "wss"
	} // 舊設定只升級，不允許降級到明文。
	return address.String(), nil
}

// 自訂 HTTPS 埠可透過環境變數設定，或直接將站台 signal 設為 HTTPS 網址。
func SignalHTTPSURL(value string) (string, error) {
	secure, err := SecureSignalURL(value)
	if err != nil {
		return "", err
	}
	address, _ := url.Parse(secure)
	if address.Scheme != "https" {
		port := os.Getenv("YOURDESK_SIGNAL_HTTPS_PORT")
		if port == "" {
			port = "8081"
		}
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return "", errors.New("HTTPS 備援埠須為 1～65535")
		}
		address.Host = net.JoinHostPort(address.Hostname(), port)
	}
	address.Scheme, address.Path, address.RawPath, address.RawQuery, address.Fragment = "https", "", "", "", ""
	return address.String(), nil
}
func TLSHTTPClient(timeout time.Duration) (*http.Client, error) {
	roots, err := x509.SystemCertPool()
	if err != nil {
		roots = x509.NewCertPool()
	}
	if len(strings.TrimSpace(string(serverTrust))) > 0 && !roots.AppendCertsFromPEM(serverTrust) {
		return nil, errors.New("內嵌 TLS 公開憑證無效")
	}
	if path := os.Getenv("YOURDESK_TLS_CA"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if !roots.AppendCertsFromPEM(data) {
			return nil, errors.New("指定的 TLS 公開憑證無效")
		}
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots}
	return &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if r.URL.Scheme != "https" {
			return errors.New("禁止 TLS 連線降級")
		}
		if len(via) >= 5 {
			return errors.New("TLS 重新導向次數過多")
		}
		return nil
	}}, nil
}

// 啟動腳本使用 Go TLS，避免系統 curl 不支援 TLS 1.3。
func CheckSignalServer(value string) error {
	client, err := TLSHTTPClient(8 * time.Second)
	if err != nil {
		return err
	}
	defer client.CloseIdleConnections()
	response, err := SignalHTTPRequest(context.Background(), client, value, "GET", "/healthz", nil)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent || response.Header.Get("X-YourDesk-Server") != "true" {
		return errors.New("TLS 配對伺服器健康檢查失敗")
	}
	return nil
}
