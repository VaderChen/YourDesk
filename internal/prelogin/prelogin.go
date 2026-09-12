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
	switch c.CodecGoal {
	case "", "balanced", "low-latency", "bandwidth":
	default:
		return errors.New("不支援的串流偏好")
	}
	switch c.Codec {
	case "auto", "hardware-h264", "hardware-hevc", "software-jpeg", "software", "":
	default:
		return errors.New("不支援的影像格式")
	}
	return nil
}

type State struct {
	Room      string `json:"room,omitempty"`
	Supported bool   `json:"supported"`
	Enabled   bool   `json:"enabled"`
	Busy      bool   `json:"busy"`
	Message   string `json:"message"`
	Error     string `json:"error,omitempty"`
}

// Handle 在一般 APP／Host 啟動前處理專用服務入口。
func Handle(ctx context.Context, args []string) (bool, error) {
	if len(args) < 2 || args[0] != "--prelogin" {
		return false, nil
	}
	switch args[1] {
	case "status", "disconnect":
		value, err := brokerRequest(ctx, args[1])
		if err == nil {
			err = json.NewEncoder(os.Stdout).Encode(value)
		}
		return true, err
	case "daemon":
		return true, runDaemon(ctx)
	case "agent":
		return true, runAgent(ctx)
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
