package clientui

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoteAudioTogglePersistsOnlyAudio(t *testing.T) {
	dir := t.TempDir()
	s := &server{configPath: filepath.Join(dir, "sites.json"), preferences: Preferences{AudioCodec: "auto", Codec: "hardware-hevc", Language: "ja", SourceFPSLimit: 30, MCPEnabled: true}}
	original := s.preferences
	for _, enabled := range []bool{true, false} {
		if err := s.toggleRemoteAudio(); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(dir, "preferences.json"))
		if err != nil {
			t.Fatal(err)
		}
		var stored Preferences
		if err := json.Unmarshal(data, &stored); err != nil {
			t.Fatal(err)
		}
		if stored.RemoteAudio != enabled || s.preferences.RemoteAudio != enabled {
			t.Fatal("聲音開關未同步保存")
		}
		stored.RemoteAudio = original.RemoteAudio
		a, _ := json.Marshal(stored)
		b, _ := json.Marshal(original)
		if string(a) != string(b) {
			t.Fatal("快捷開關改動了其他偏好")
		}
	}
	s.configPath = filepath.Join(dir, "missing", "sites.json")
	if err := s.toggleRemoteAudio(); err == nil {
		t.Fatal("應回報儲存失敗")
	}
	if s.preferences.RemoteAudio {
		t.Fatal("儲存失敗不應改變實際開關")
	}
}

func TestPreferencesPatchPreservesAudioToggle(t *testing.T) {
	s := &server{configPath: filepath.Join(t.TempDir(), "sites.json"), token: "test", preferences: Preferences{AudioCodec: "opus", Codec: "auto", CodecGoal: "balanced", Language: "en", Theme: "light", SourceFPSLimit: 20, BitrateLimitMbps: 12}}
	if err := s.toggleRemoteAudio(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("PUT", "/api/preferences", strings.NewReader(`{"disableHints":true}`))
	request.Header.Set("X-YourDesk-Token", "test")
	response := httptest.NewRecorder()
	s.api(response, request)
	if response.Code != 200 {
		t.Fatalf("%d: %s", response.Code, response.Body.String())
	}
	if !s.preferences.RemoteAudio || !s.preferences.DisableHints {
		t.Fatal("部分設定更新覆蓋聲音快捷開關")
	}
}
