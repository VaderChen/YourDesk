//go:build darwin && cgo

package prelogin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func UpdateStreamSettings(ctx context.Context, settings StreamSettings) error {
	if err := settings.Validate(); err != nil {
		return err
	}
	// 沿用受保護 helper 的 UID／PID 驗證，不要求重新安裝或管理員授權。
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	data, err := exec.CommandContext(ctx, executable, "--prelogin", "stream-settings", settings.Codec, settings.CodecGoal).Output()
	if err != nil {
		return errors.New(streamSettingsUnavailable)
	}
	return readStreamSettingsReply(bytes.NewReader(data))
}

func requestStreamSettings(ctx context.Context, settings StreamSettings) error {
	if err := settings.Validate(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", socket)
	if err != nil {
		return errors.New("登入前服務尚未就緒，請稍後重試。")
	}
	defer conn.Close()
	uid, pid, err := peerIdentity(conn.(*net.UnixConn))
	if err != nil || uid != 0 || processPath(pid) != executable {
		return errors.New("無法驗證系統服務")
	}
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)
	if err = json.NewEncoder(conn).Encode(struct {
		Operation string         `json:"operation"`
		Stream    StreamSettings `json:"stream"`
	}{"stream-settings", settings}); err != nil {
		return err
	}
	return readStreamSettingsReply(conn)
}

func saveStreamConfig(path string, config Config) error {
	data, err := json.Marshal(config)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".stream-config-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err = file.Write(data); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
