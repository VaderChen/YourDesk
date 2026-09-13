package clientui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"yourdesk/internal/security"
)

func TestSiriBridgeSmoke(t *testing.T) {
	dir := t.TempDir()
	s := &server{configPath: filepath.Join(dir, "sites.json"), origin: "http://127.0.0.1:1234", token: "private-ui-token", automationShow: make(chan string, 1), children: map[string]*process{}, library: Library{Sites: []Site{{ID: "one", Name: "辦公室", Room: "123", Note: "不可洩漏"}, {ID: "two", Name: "家中", Room: "456"}}}}
	mux := http.NewServeMux()
	stop, err := s.installAutomation(mux)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	path := filepath.Join(dir, "siri-bridge.json")
	stat, err := os.Stat(path)
	if err != nil || stat.Mode().Perm() != 0600 {
		t.Fatal("能力憑證權限錯誤", err)
	}
	data, _ := os.ReadFile(path)
	var ticket map[string]any
	if err = json.Unmarshal(data, &ticket); err != nil {
		t.Fatal(err)
	}
	token := ticket["token"].(string)
	call := func(action, id, auth, origin string) *httptest.ResponseRecorder {
		b, _ := json.Marshal(siriRequest{Action: action, ID: id})
		r := httptest.NewRequest("POST", "/automation", bytes.NewReader(b))
		r.Header.Set("Authorization", auth)
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	for _, tc := range []struct{ auth, origin string }{{"", ""}, {"Bearer private-ui-token", ""}, {"Bearer " + token, "https://example.com"}} {
		if w := call("sites", "", tc.auth, tc.origin); w.Code != 403 {
			t.Fatal("未阻擋未授權來源")
		}
	}
	auth := "Bearer " + token
	if w := call("sites", "", auth, ""); w.Code != 200 || strings.Contains(w.Body.String(), "不可洩漏") || strings.Contains(w.Body.String(), "123") {
		t.Fatal(w.Body.String())
	}
	if w := call("open", "", auth, ""); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	<-s.automationShow
	if w := call("show-site", "one", auth, ""); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if <-s.automationShow != "123" {
		t.Fatal("開啟錯誤站台")
	}
	if w := call("connect", "one", auth, ""); w.Code != 400 || !strings.Contains(w.Body.String(), "記住密碼") {
		t.Fatal("缺少密碼未拒絕", w.Body.String())
	}
	<-s.automationShow
	if w := call("status", "", auth, ""); w.Code != 200 || !strings.Contains(w.Body.String(), "沒有") {
		t.Fatal(w.Body.String())
	}
	s.executable = filepath.Join(dir, "client")
	if err := os.WriteFile(filepath.Join(dir, "yourdesk-remote"), []byte("#!/bin/sh\nwhile IFS= read -r line; do :; done\n"), 0700); err != nil {
		t.Fatal(err)
	}
	secret, err := security.NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	if err := saveJSON(filepath.Join(dir, "credentials.json"), map[string]string{credentialID("", "123"): secret}); err != nil {
		t.Fatal(err)
	}
	if w := call("connect", "one", auth, ""); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	s.mu.Lock()
	child := s.children["viewer:one"]
	s.mu.Unlock()
	if child == nil {
		t.Fatal("未經正式 API 啟動 Remote")
	}
	defer child.cmd.Process.Kill()
	if w := call("disconnect", "one", auth, ""); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	select {
	case <-child.done:
	case <-time.After(3 * time.Second):
		t.Fatal("未結束 Remote")
	}
	s.children["viewer:one"] = &process{siteID: "one", stage: "connected"}
	s.children["viewer:two"] = &process{siteID: "two", stage: "connecting"}
	if w := call("status", "one", auth, ""); w.Code != 200 || !strings.Contains(w.Body.String(), "已連線") || strings.Contains(w.Body.String(), "家中") {
		t.Fatal(w.Body.String())
	}
	if w := call("disconnect", "", auth, ""); w.Code != 400 {
		t.Fatal("多連線未要求指定")
	}
	if w := call("disconnect", "missing", auth, ""); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w := call("shell", "", auth, ""); w.Code != 400 {
		t.Fatal("接受任意操作")
	}
	stop()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("未移除憑證")
	}
}
