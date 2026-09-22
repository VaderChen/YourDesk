package clientui

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"yourdesk/internal/prelogin"
)

func TestManagedCodecPreferencesSmoke(t *testing.T) {
	for _, scenario := range []string{"codec", "goal", "audio", "rejected", "busy", "network", "disk-failure", "unmanaged"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			s := &server{configPath: filepath.Join(dir, "sites.json"), preferences: Preferences{Codec: "auto", CodecGoal: "balanced", AudioCodec: "opus"}}
			original := s.preferences
			next := original
			managed := true
			wantCalls, wantError := 1, false
			switch scenario {
			case "codec", "rejected", "busy", "disk-failure", "unmanaged":
				next.Codec = "hardware-hevc"
			case "goal":
				next.CodecGoal = "bandwidth"
			case "audio":
				next.AudioCodec = "aac"
				wantCalls = 0
			case "network":
				next.DirectListen = true
				wantCalls, wantError = 0, true
			}
			if scenario == "busy" {
				s.preloginBusy = true
				wantCalls, wantError = 0, true
			}
			if scenario == "disk-failure" {
				s.configPath = filepath.Join(dir, "missing", "sites.json")
				wantCalls, wantError = 0, true
			}
			if scenario == "rejected" {
				wantError = true
			}
			if scenario == "unmanaged" {
				managed, wantCalls = false, 0
			}
			calls := 0
			err := s.storePreferences(context.Background(), next, managed, func(_ context.Context, settings prelogin.StreamSettings) error {
				calls++
				if settings.Codec != next.Codec || settings.CodecGoal != next.CodecGoal {
					t.Fatal("送往服務的編碼與使用者選擇不同")
				}
				if scenario == "rejected" {
					return errors.New("服務拒絕變更")
				}
				return nil
			})
			if (err != nil) != wantError || calls != wantCalls {
				t.Fatalf("err=%v, calls=%d", err, calls)
			}
			want := next
			if wantError {
				want = original
			}
			if s.preferences.Codec != want.Codec || s.preferences.CodecGoal != want.CodecGoal || s.preferences.AudioCodec != want.AudioCodec || s.preferences.DirectListen != want.DirectListen {
				t.Fatal("記憶體偏好與操作結果不一致")
			}
			if !wantError || scenario == "rejected" {
				data, readErr := os.ReadFile(filepath.Join(dir, "preferences.json"))
				var stored Preferences
				if readErr != nil || json.Unmarshal(data, &stored) != nil || stored.Codec != want.Codec || stored.CodecGoal != want.CodecGoal || stored.AudioCodec != want.AudioCodec {
					t.Fatal("持久化偏好未保存或未還原")
				}
			}
		})
	}
}
