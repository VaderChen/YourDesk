package prelogin

import (
	"encoding/json"
	"errors"
	"io"
)

// StreamSettings 僅允許變更編碼，不接受配對資料、執行路徑或網路監聽設定。
type StreamSettings struct {
	Codec     string `json:"codec"`
	CodecGoal string `json:"codecGoal"`
}

func (s StreamSettings) Validate() error {
	switch s.CodecGoal {
	case "", "balanced", "low-latency", "bandwidth":
	default:
		return errors.New("不支援的串流偏好")
	}
	switch s.Codec {
	case "", "auto", "hardware-h264", "hardware-hevc", "hardware-av1", "software-av1", "hardware-jpeg", "software-jpeg", "software":
	default:
		return errors.New("不支援的影像格式")
	}
	return nil
}

const streamSettingsUnavailable = "登入前服務尚未支援同步編碼設定，請更新 APP 與服務後再試。"

type streamSettingsReply struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func readStreamSettingsReply(r io.Reader) error {
	var reply streamSettingsReply
	if err := json.NewDecoder(io.LimitReader(r, 4096)).Decode(&reply); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New(streamSettingsUnavailable)
		}
		return errors.New("無法確認服務編碼設定，請稍後重試。")
	}
	if reply.Error != "" {
		return errors.New(reply.Error)
	}
	if !reply.OK {
		return errors.New(streamSettingsUnavailable)
	}
	return nil
}

// 先持久化，成功才改變執行中的設定。相同請求不重啟 Host。
func applyStreamSettings(current *Config, settings StreamSettings, save func(Config) error) (bool, error) {
	if err := settings.Validate(); err != nil {
		return false, err
	}
	next := *current
	next.Codec, next.CodecGoal = settings.Codec, settings.CodecGoal
	if next == *current {
		return false, nil
	}
	if err := save(next); err != nil {
		return false, errors.New("無法儲存服務編碼設定，請稍後重試。")
	}
	*current = next
	return true, nil
}
