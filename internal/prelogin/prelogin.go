// Package prelogin 管理未登入連線的安裝與工作階段生命週期。
package prelogin

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"yourdesk/internal/security"
)

type Config struct {
	Tailcat   bool   `json:"tailcat,omitempty"`
	Room      string `json:"room"`
	Signal    string `json:"signal"`
	Secret    string `json:"secret"`
	Codec     string `json:"codec"`
	CodecGoal string `json:"codecGoal,omitempty"`
	Direct    bool   `json:"direct"`
}

func (c Config) validate() error {
	if len(c.Room) == 0 || len(c.Room) > 200 {
		return errors.New("無效的裝置 ID")
	}
	if _, err := security.SecureSignalURL(c.Signal); err != nil {
		return err
	}
	if _, err := security.DecodeSecret(c.Secret); err != nil {
		return err
	}
	return (StreamSettings{Codec: c.Codec, CodecGoal: c.CodecGoal}).Validate()
}

type State struct {
	SASDisabled  bool   `json:"sasDisabled"`
	SASSupported bool   `json:"sasSupported"`
	SASAllowed   bool   `json:"sasAllowed"`
	Room         string `json:"room,omitempty"`
	Supported    bool   `json:"supported"`
	Enabled      bool   `json:"enabled"`
	Busy         bool   `json:"busy"`
	Message      string `json:"message"`
	Error        string `json:"error,omitempty"`
}

// Handle 在一般 APP／Host 啟動前處理專用服務入口。
func Handle(ctx context.Context, args []string) (bool, error) {
	if len(args) < 2 || args[0] != "--prelogin" {
		return false, nil
	}
	switch args[1] {
	case "stream-settings":
		if len(args) != 4 {
			return true, errors.New("缺少編碼設定")
		}
		err := requestStreamSettings(ctx, StreamSettings{Codec: args[2], CodecGoal: args[3]})
		reply := streamSettingsReply{OK: err == nil}
		if err != nil {
			reply.Error = err.Error()
		}
		return true, json.NewEncoder(os.Stdout).Encode(reply)
	case "status", "disconnect", "health":
		value, err := brokerRequest(ctx, args[1])
		if err == nil && args[1] == "health" && !value {
			err = errors.New("登入前服務尚未就緒")
		}
		if err == nil {
			err = json.NewEncoder(os.Stdout).Encode(value)
		}
		return true, err
	case "daemon":
		return true, runDaemon(ctx)
	case "agent":
		return true, runAgent(ctx)
	case "update-worker":
		if len(args) != 3 {
			return true, errors.New("缺少服務更新工作")
		}
		return true, runUpdateWorker(ctx, args[2])
	case "sas-enable":
		if len(args) != 2 {
			return true, errors.New("無效的 SAS 授權參數")
		}
		return true, enableSecureAttentionPolicy()
	case "install-sas":
		if !Status().SASSupported {
			return true, errors.New("此平台不支援 Windows SAS 授權。")
		}
		if len(args) != 3 {
			return true, errors.New("缺少安裝設定")
		}
		if err := install(args[2]); err != nil {
			return true, err
		}
		return true, enableSecureAttentionPolicy()
	case "install":
		if len(args) != 3 {
			return true, errors.New("缺少安裝設定")
		}
		return true, install(args[2])
	case "remove":
		return true, remove()
	default:
		return true, errors.New("未知服務操作")
	}
}
