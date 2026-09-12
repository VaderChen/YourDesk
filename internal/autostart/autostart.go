// Package autostart 管理目前使用者登入後的啟動項目，不安裝系統服務。
package autostart

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"yourdesk/internal/security"
)

const Hint = "登入系統後自動啟動 YourDesk。"
const Unsupported = "目前環境不支援登入後自動啟動。"
const Failed = "無法變更自動啟動，請確認安裝位置與使用者權限。"
const PreloginRequired = "請先開啟登入後自動啟動，再啟用登入前連線。"
const PreloginActive = "請先關閉登入前連線，再關閉自動啟動。"

type State struct {
	Enabled   bool   `json:"enabled"`
	Supported bool   `json:"supported"`
	Message   string `json:"message"`
}

type Config struct {
	Executable string
	Args       []string
	Desktop    bool
}

var changeMu sync.Mutex

// 啟動項目只保存固定參數，不複製密碼、父管線或任意命令列。
func ClientConfig(signal, room string, desktop bool) (Config, error) {
	if _, err := security.SecureSignalURL(signal); err != nil {
		return Config{}, err
	}
	executable, err := os.Executable()
	if err != nil {
		return Config{}, err
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return Config{}, err
	}
	c := Config{Executable: executable, Desktop: desktop, Args: []string{"-signal", signal}}
	if desktop {
		c.Args = append(c.Args, "-ui")
	}
	if room != "" {
		c.Args = append(c.Args, "-room", room)
	}
	return c, nil
}

func Status(desktop bool) State {
	changeMu.Lock()
	defer changeMu.Unlock()
	enabled, err := enabled(desktop)
	s := State{Enabled: enabled, Supported: supported(desktop), Message: Hint}
	if !s.Supported {
		s.Message = Unsupported
	}
	if err != nil {
		s.Message = Failed
	}
	return s
}

func Configure(ctx context.Context, on bool, c Config) error {
	changeMu.Lock()
	defer changeMu.Unlock()
	if !on {
		return configure(ctx, false, c)
	}
	if !supported(c.Desktop) {
		return errors.New(Unsupported)
	}
	if !filepath.IsAbs(c.Executable) {
		return errors.New(Failed)
	}
	if st, err := os.Stat(c.Executable); err != nil || !st.Mode().IsRegular() {
		return errors.New(Failed)
	}
	for _, value := range append([]string{c.Executable}, c.Args...) {
		if strings.ContainsAny(value, "\x00\r\n") {
			return errors.New(Failed)
		}
	}
	return configure(ctx, true, c)
}

// 同目錄替換，避免寫入中斷留下不完整的登入項目。
func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".yourdesk-autostart-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
func removeFile(path string) error {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
