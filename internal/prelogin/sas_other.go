//go:build !windows

package prelogin

import (
	"context"
	"errors"
)

func SecureAttentionAvailable() bool { return false }
func SendSecureAttention(context.Context) error {
	return errors.New("Ctrl+Alt+Del 僅支援遠端 Windows 登入前服務。")
}

func ConfigureSecureAttention(context.Context, Config) error {
	return errors.New("此平台不需要 Windows SAS 授權。")
}
func enableSecureAttentionPolicy() error {
	return errors.New("此平台不支援 Windows SAS 授權。")
}
