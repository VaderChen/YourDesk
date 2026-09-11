package clipboard

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
)

// 序號只作為重新檢查的訊號；系統重新發布相同格式不應中斷既有快照。
func (s *Sync) transferSourceChanged(source content, files map[string]os.FileInfo, revision *int64) (bool, error) {
	s.nativeMu.Lock()
	current := nativeRevision()
	if current == *revision {
		s.nativeMu.Unlock()
		return false, nil
	}
	s.nativeMu.Unlock()
	value, current, err := readStableContent(readContent)
	s.nativeMu.Lock()
	defer s.nativeMu.Unlock()
	if err != nil {
		return false, err
	}
	if current != nativeRevision() {
		return false, errors.New("檢查期間剪貼簿再次變更")
	}
	if s.sentValid && current == s.sent {
		return true, nil
	}
	if value.Kind != source.Kind || !bytes.Equal(value.Data, source.Data) || len(value.Paths) != len(source.Paths) {
		slog.Info("剪貼簿內容已更換，取消舊傳輸", "原類型", source.Kind, "新類型", value.Kind, "原項目", len(source.Paths), "新項目", len(value.Paths))
		return true, nil
	}
	seen := map[string]bool{}
	for _, path := range value.Paths {
		name := filepath.Clean(path)
		before, ok := files[name]
		if !ok || seen[name] {
			return true, nil
		}
		seen[name] = true
		after, e := os.Lstat(path)
		if e != nil {
			return true, nil
		}
		if !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
			return true, nil
		}
	}
	*revision = current
	s.observed = current
	return false, nil
}
