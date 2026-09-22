package prelogin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	winio "github.com/tailscale/go-winio"
)

var sasToggleMu sync.RWMutex

// 此標記只限制 YourDesk，不撤銷其他應用共用的 Windows SAS 原則。
func secureAttentionDisabled() bool {
	root, _, err := servicePaths()
	if err != nil {
		return true
	}
	_, err = os.Stat(filepath.Join(root, "sas.disabled"))
	return !errors.Is(err, os.ErrNotExist)
}

func setSecureAttentionEnabled(enabled bool) error {
	sasToggleMu.Lock()
	defer sasToggleMu.Unlock()
	root, _, err := servicePaths()
	if err != nil {
		return err
	}
	path := filepath.Join(root, "sas.disabled")
	if err = noReparsePath(path); err != nil {
		return err
	}
	if enabled {
		if !secureAttentionPolicyAllowed() {
			return errors.New("Windows 原則仍未允許服務產生 Ctrl+Alt+Del，請洽系統管理員。")
		}
		err = os.Remove(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	err = secureWrite(path, nil, "D:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;GR;;;AU)")
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	return err
}

func SetSecureAttentionEnabled(ctx context.Context, enabled bool) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	conn, err := winio.DialPipeContext(ctx, controlPipe)
	if err != nil {
		return errors.New("登入前服務尚未就緒，請稍後重試。")
	}
	defer conn.Close()
	if err = verifyControlServer(conn); err != nil {
		return err
	}
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)
	if err = json.NewEncoder(conn).Encode(map[string]any{"operation": "sas-settings", "enabled": enabled}); err != nil {
		return err
	}
	var reply streamSettingsReply
	if err = json.NewDecoder(io.LimitReader(conn, 4096)).Decode(&reply); err != nil || (!reply.OK && reply.Error == "") {
		return errors.New("登入前服務尚未支援 Ctrl+Alt+Del 開關，請更新 APP 與服務後再試。")
	}
	if reply.Error != "" {
		return errors.New(reply.Error)
	}
	return nil
}
