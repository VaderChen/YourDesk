//go:build darwin && cgo

package prelogin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStreamConfigPersistenceSmoke(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	for _, codec := range []string{"auto", "hardware-hevc"} {
		want := Config{Room: "room", Secret: "private-secret", Codec: codec}
		if err := saveStreamConfig(path, want); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var got Config
		if json.Unmarshal(data, &got) != nil || got != want {
			t.Fatal("設定未完整持久化")
		}
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("服務設定必須維持 0600")
		}
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatal("留下含配對資料的暫存檔")
	}
}
