package clientui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"yourdesk/internal/prelogin"
)

// 呼叫端持有 s.mu；服務啟停、Host 重啟及偏好儲存不得交錯。
func (s *server) storePreferences(ctx context.Context, preferences Preferences, managed bool, updateStream func(context.Context, prelogin.StreamSettings) error) error {
	codecChanged := preferences.Codec != s.preferences.Codec || preferences.CodecGoal != s.preferences.CodecGoal
	networkChanged := preferences.DirectListen != s.preferences.DirectListen || preferences.TailcatEnabled != s.preferences.TailcatEnabled
	if s.preloginBusy && (codecChanged || networkChanged) {
		return errors.New("未登入服務正在變更，請稍候。")
	}
	if managed && networkChanged {
		return errors.New("請先停用未登入開機，再修改 IP 直連或 Tailcat 模式。")
	}
	startedMCP := false
	if preferences.MCPEnabled && s.mcpStop == nil {
		stop, err := s.startMCP(s.updater.ctx)
		if err != nil {
			return fmt.Errorf("MCP 無法啟動：%w", err)
		}
		s.mcpStop = stop
		startedMCP = true
	}
	committed := false
	defer func() {
		if !committed && startedMCP {
			s.mcpStop()
			s.mcpStop = nil
		}
	}()
	path := filepath.Join(filepath.Dir(s.configPath), "preferences.json")
	if err := saveJSON(path, preferences); err != nil {
		return err
	}
	if managed && codecChanged {
		settings := prelogin.StreamSettings{Codec: preferences.Codec, CodecGoal: preferences.CodecGoal}
		if err := updateStream(ctx, settings); err != nil {
			// 未確認服務已套用時，不回報成功或留下新的本機偏好。
			if rollbackErr := saveJSON(path, s.preferences); rollbackErr != nil {
				return fmt.Errorf("%w；還原本機偏好失敗：%v", err, rollbackErr)
			}
			return err
		}
	}
	if !preferences.MCPEnabled && s.mcpStop != nil {
		s.mcpStop()
		s.mcpStop = nil
	}
	s.preferences = preferences
	committed = true
	if !managed && (codecChanged || networkChanged) {
		if host := s.children["host"]; host != nil {
			_ = host.cmd.Process.Kill()
		}
	}
	return nil
}

// 呼叫端持有 s.mu；僅更新聲音開關，避免覆蓋其他設定或重啟 Host。
func (s *server) toggleRemoteAudio() error {
	preferences := s.preferences
	preferences.RemoteAudio = !preferences.RemoteAudio
	if err := saveJSON(filepath.Join(filepath.Dir(s.configPath), "preferences.json"), preferences); err != nil {
		return err
	}
	s.preferences = preferences
	return nil
}
