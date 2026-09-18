package clientui

import (
	"context"
	"path/filepath"
	"testing"
)

func TestPendingInstalledNotesSurviveNewReleaseCheck(t *testing.T) {
	t.Setenv("YOURDESK_TEST_UPDATE", "")
	t.Setenv("YOURDESK_TEST_UPDATE_NOTES", "")
	dir := t.TempDir()
	// 已安裝版說明尚未確認，但背景檢查已找到下一版。
	state := updateStatus{Version: "1.99.1231 build 2359", Available: true, NotesVersion: currentVersion(), ShowNotes: true, Notes: map[string][]string{"zh-Hant": {"本版更新"}}}
	if err := saveJSON(filepath.Join(dir, "updates.json"), state); err != nil {
		t.Fatal(err)
	}
	u := newUpdateManager(context.Background(), dir)
	got := u.snapshot()
	if !got.ShowNotes || got.NotesVersion != currentVersion() || got.Notes["zh-Hant"][0] != "本版更新" {
		t.Fatalf("未保留已安裝版說明：%+v", got)
	}
	if !got.Available {
		t.Fatal("遺失下一版更新")
	}
	u.mu.Lock()
	u.state.ShowNotes = false
	u.state.Notes = nil
	u.state.NotesVersion = ""
	u.saveLocked()
	u.mu.Unlock()
	if newUpdateManager(context.Background(), dir).snapshot().ShowNotes {
		t.Fatal("已確認的說明再次顯示")
	}
}
