package clientui

import (
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

// 版本化的站台分享資料不包含連線密碼、UI token 或其他本機設定。
func siteShareURI(name, room, signal string) (string, error) {
	name, room = strings.TrimSpace(name), strings.TrimSpace(room)
	endpoint, err := url.Parse(signal)
	if err != nil || !validSignal(signal) || endpoint.RawQuery != "" || len(room) == 0 || len(room) > 200 || len(name) > 200 {
		return "", errors.New("目前站台資料無法產生 QR Code")
	}
	if name == "" {
		name = room
	}
	uri := url.URL{Scheme: "yourdesk", Host: "site", RawQuery: url.Values{"v": {"1"}, "name": {name}, "room": {room}, "signal": {signal}}.Encode()}
	payload := uri.String()
	if len(payload) > 1800 {
		return "", errors.New("站台資料過長，無法產生 QR Code")
	}
	return payload, nil
}

func (s *server) siteQR(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	name, room, signal := s.info["hostname"], s.info["room"], s.options.Signal
	s.mu.Unlock()
	payload, err := siteShareURI(name, room, signal)
	if err != nil {
		fail(w, err)
		return
	}
	png, err := qrcode.Encode(payload, qrcode.Medium, 512)
	if err != nil {
		fail(w, errors.New("QR Code 產生失敗"))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	respond(w, 200, map[string]string{"name": name, "room": room, "uri": payload, "image": "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)})
}
