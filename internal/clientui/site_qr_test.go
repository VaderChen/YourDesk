package clientui

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image/png"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSiteQRShare(t *testing.T) {
	s := &server{token: "private-ui-token", origin: "http://localhost:1234", info: map[string]string{"hostname": "辦公室 & Mac", "room": "YD-4U3T-KO7Y-UHKE-MHFI-A6SA"}, options: Options{Signal: "wss://desktop.mars-cloud.com:8080/ws"}}
	request := httptest.NewRequest("GET", "/api/site-qr", nil)
	denied := httptest.NewRecorder()
	s.api(denied, request)
	if denied.Code != 403 {
		t.Fatal("未授權請求取得站台資料")
	}
	request.Header.Set("X-YourDesk-Token", s.token)
	response := httptest.NewRecorder()
	s.api(response, request)
	if response.Code != 200 || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(response.Code, response.Body.String())
	}
	var out map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(out["uri"])
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if u.Scheme != "yourdesk" || u.Host != "site" || q.Get("v") != "1" || q.Get("name") != s.info["hostname"] || q.Get("room") != s.info["room"] || q.Get("signal") != s.options.Signal || len(q) != 4 {
		t.Fatal("分享欄位無法往返")
	}
	if strings.Contains(response.Body.String(), s.token) {
		t.Fatal("UI token 外洩")
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(out["image"], "data:image/png;base64,"))
	if err != nil {
		t.Fatal(err)
	}
	info, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil || info.Width != 512 || info.Height != 512 {
		t.Fatal("QR PNG 格式無效", err)
	}
	if dir := os.Getenv("YOURDESK_QR_SMOKE_DIR"); dir != "" {
		if err := os.WriteFile(filepath.Join(dir, "site-qr.png"), data, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "expected.txt"), []byte(out["uri"]), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, signal := range []string{"wss://user:pass@host/ws", "wss://host/ws?token=secret", "javascript:alert(1)"} {
		if _, err := siteShareURI("name", "room", signal); err == nil {
			t.Fatal("不應分享含憑證或無效的伺服器 URL")
		}
	}
	if _, err := siteShareURI("name", "", s.options.Signal); err == nil {
		t.Fatal("接受空白 ID")
	}
}
