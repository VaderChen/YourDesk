package security

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type localSecret struct {
	Secret      string `json:"secret"`
	NeedsChange bool   `json:"needsChange"`
	Version     int    `json:"version"`
}

const localSecretVersion = 2

func localSecretPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "YourDesk", "client-secret.json"), nil
}
func LocalSecret(explicit string) (string, bool, error) {
	if explicit != "" {
		_, err := DecodeSecret(explicit)
		return explicit, false, err
	}
	path, err := localSecretPath()
	if err != nil {
		return "", false, err
	}
	if data, err := os.ReadFile(path); err == nil {
		var value localSecret
		if err = json.Unmarshal(data, &value); err != nil {
			return "", false, err
		}
		if value.Version == localSecretVersion || (value.Version == 1 && !value.NeedsChange) {
			_, err = DecodeSecret(value.Secret)
			if err == nil && value.Version != localSecretVersion {
				err = SaveLocalSecret(value.Secret, false)
			}
			return value.Secret, value.NeedsChange, err
		}
		// 舊格式或尚未更改的六碼初始密碼不再沿用；統一換發高強度隨機密碼。
	} else if !os.IsNotExist(err) {
		return "", false, err
	}
	secret, err := NewSecret()
	if err != nil {
		return "", false, err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", false, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return "", false, err
	}
	err = json.NewEncoder(file).Encode(localSecret{Secret: secret, NeedsChange: true, Version: localSecretVersion})
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	return secret, true, err
}
func SaveLocalSecret(secret string, needsChange bool) error {
	if _, err := DecodeSecret(secret); err != nil {
		return err
	}
	path, err := localSecretPath()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".client-secret-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err = json.NewEncoder(file).Encode(localSecret{Secret: secret, NeedsChange: needsChange, Version: localSecretVersion}); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
