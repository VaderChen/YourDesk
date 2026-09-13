package security

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// 健康檢查、在線查詢與配對採用相同的 HTTPS 備援位址規則。
func SignalHTTPRequest(ctx context.Context, client *http.Client, signal, method, path string, body []byte) (*http.Response, error) {
	secure, err := SecureSignalURL(signal)
	if err != nil {
		return nil, err
	}
	primary, _ := url.Parse(secure)
	primary.Scheme, primary.Path, primary.RawPath, primary.RawQuery, primary.Fragment = "https", path, "", "", ""
	addresses := []string{primary.String()}
	if fallback, e := SignalHTTPSURL(signal); e == nil && fallback+path != primary.String() {
		addresses = append(addresses, fallback+path)
	}
	for _, address := range addresses {
		request, e := http.NewRequestWithContext(ctx, method, address, bytes.NewReader(body))
		if e != nil {
			return nil, e
		}
		request.Header.Set("Content-Type", "application/json")
		response, e := client.Do(request)
		if e != nil {
			err = e
			if ctx.Err() != nil {
				break
			}
			continue
		}
		if (path == "/healthz" && response.StatusCode == 204 && response.Header.Get("X-YourDesk-Server") == "true") || (path != "/healthz" && response.StatusCode == 200) {
			return response, nil
		}
		err = fmt.Errorf("訊號 HTTPS 回應：%s", response.Status)
		response.Body.Close()
	}
	return nil, err
}
