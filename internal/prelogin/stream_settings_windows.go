package prelogin

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net"
	"os"
	"time"

	winio "github.com/tailscale/go-winio"
	"golang.org/x/sys/windows"
)

func UpdateStreamSettings(ctx context.Context, settings StreamSettings) error {
	return requestStreamSettings(ctx, settings)
}

func requestStreamSettings(ctx context.Context, settings StreamSettings) error {
	if err := settings.Validate(); err != nil {
		return err
	}
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
	if err = json.NewEncoder(conn).Encode(struct {
		Operation string         `json:"operation"`
		Stream    StreamSettings `json:"stream"`
	}{"stream-settings", settings}); err != nil {
		return err
	}
	return readStreamSettingsReply(conn)
}

func authorizeConsoleSettings(conn net.Conn) error {
	identity, err := updateClient(conn)
	if err != nil {
		return err
	}
	if identity.Session == 0 || identity.Session == 0xffffffff || identity.Session != windows.WTSGetActiveConsoleSessionId() {
		return errors.New("請在目前主控台修改服務設定。")
	}
	return nil
}

func saveStreamConfig(path string, config Config) error {
	if err := noReparsePath(path); err != nil {
		return err
	}
	data, err := json.Marshal(config)
	if err != nil {
		return err
	}
	temp := path + "." + rand.Text() + ".tmp"
	defer os.Remove(temp)
	if err = secureWrite(temp, data, "D:P(A;;FA;;;SY)(A;;FA;;;BA)"); err != nil {
		return err
	}
	source, err := windows.UTF16PtrFromString(temp)
	if err != nil {
		return err
	}
	target, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(source, target, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
