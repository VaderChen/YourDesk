package clientui

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// 匯出可攜的站台資料；檔名唯一，避免覆寫使用者既有備份。
func exportLibrary(library Library) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Downloads")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	exported := publicLibrary(library)
	if exported.Groups == nil {
		exported.Groups = []Group{}
	}
	if exported.Sites == nil {
		exported.Sites = []Site{}
	}
	data, err := json.MarshalIndent(exported, "", "  ")
	if err != nil {
		return "", err
	}
	file, err := os.CreateTemp(dir, "YourDesk-sites-*.json")
	if err != nil {
		return "", err
	}
	ok := false
	defer func() {
		file.Close()
		if !ok {
			os.Remove(file.Name())
		}
	}()
	if _, err = file.Write(data); err != nil {
		return "", err
	}
	if err = file.Sync(); err != nil {
		return "", err
	}
	if err = file.Close(); err != nil {
		return "", err
	}
	ok = true
	return file.Name(), nil
}
