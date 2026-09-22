package clientui

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSASPromptDismissalPersistenceSmoke(t *testing.T) {
	dir := t.TempDir()
	s := &server{token: "smoke", configPath: filepath.Join(dir, "sites.json"), preferences: Preferences{
		Language: "zh-Hant", Theme: "light", SourceFPSLimit: 20, BitrateLimitMbps: 12,
		Codec: "auto", CodecGoal: "balanced", AudioCodec: "opus", SelectedGroup: "keep-group",
	}}
	put := func(body string) {
		t.Helper()
		r := httptest.NewRequest("PUT", "/api/preferences", strings.NewReader(body))
		r.Header.Set("X-YourDesk-Token", "smoke")
		w := httptest.NewRecorder()
		s.api(w, r)
		if w.Code != 200 {
			t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
		}
	}
	load := func() Preferences {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(dir, "preferences.json"))
		if err != nil {
			t.Fatal(err)
		}
		var preferences Preferences
		if err = json.Unmarshal(data, &preferences); err != nil {
			t.Fatal(err)
		}
		return preferences
	}
	put(`{"sasPromptDismissed":true}`)
	stored := load()
	if !stored.SASPromptDismissed || !s.preferences.SASPromptDismissed || stored.Codec != "auto" || stored.SelectedGroup != "keep-group" || stored.AudioCodec != "opus" {
		t.Fatal("取消提示未保存，或覆寫了其他偏好")
	}
	// 用保存檔重建程序狀態，再修改一般設定；取消選擇仍須保留。
	s = &server{token: "smoke", configPath: filepath.Join(dir, "sites.json"), preferences: stored}
	put(`{"theme":"dark"}`)
	stored = load()
	if !stored.SASPromptDismissed || stored.Theme != "dark" {
		t.Fatal("重新啟動或一般設定儲存遺失取消選擇")
	}
}
