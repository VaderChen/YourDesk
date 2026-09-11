package clientui

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
)

// 密碼獨立保存於本機私有設定檔，不回傳給 HTML 或寫入站台匯出資料。
func credentialID(signal, room string) string {
	sum := sha256.Sum256([]byte(signal + "\x00" + room))
	return hex.EncodeToString(sum[:])
}
func (s *server) credentials() (map[string]string, error) {
	values := map[string]string{}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(s.configPath), "credentials.json"))
	if os.IsNotExist(err) {
		return values, nil
	}
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(data, &values)
	if values == nil {
		values = map[string]string{}
	}
	return values, err
}
func (s *server) remembered(signal, room string) string {
	values, err := s.credentials()
	if err != nil {
		return ""
	}
	return values[credentialID(signal, room)]
}
func (s *server) remember(p *process) error {
	if p.kind == "host" || p.credentialKey == "" {
		return nil
	}
	values, err := s.credentials()
	if err != nil {
		return err
	}
	if p.remember && p.pendingSecret != "" {
		values[p.credentialKey] = p.pendingSecret
	} else {
		delete(values, p.credentialKey)
	}
	p.pendingSecret = ""
	return saveJSON(filepath.Join(filepath.Dir(s.configPath), "credentials.json"), values)
}
