package clientui

import (
	"context"
	"os"
	"time"
	"yourdesk/internal/authlog"
	"yourdesk/internal/hostguard"
)

// 啟動子程序前完成查詢；等待使用者確認時不持有介面鎖。
func (s *server) waitHostPreflight(ctx context.Context) bool {
	for ctx.Err() == nil {
		s.mu.Lock()
		room, signal := s.info["room"], s.options.Signal
		excluded := map[int]bool{os.Getpid(): true}
		for _, child := range s.children {
			if child.cmd.Process != nil {
				excluded[child.cmd.Process.Pid] = true
			}
		}
		s.mu.Unlock()
		owners, err := hostguard.FindExisting(ctx, room, signal, excluded)
		if ctx.Err() != nil {
			return false
		}
		if err != nil {
			authlog.Event("host-preflight-unavailable", map[string]any{"class": authlog.ErrorClass(err)})
			return true // 無法核對 PID 時維持原本的本機鎖與 Server 衝突警告。
		}
		if len(owners) == 0 {
			return true
		}
		owner := owners[0]
		s.mu.Lock()
		s.hostConflict = &owner
		s.hostConflictPreflight = true
		s.notice = "此裝置已有另一個 Host 連線，請結束原程式後再試。"
		s.mu.Unlock()
		ticker := time.NewTicker(time.Second)
		for hostguard.IsSameProcess(owner) {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return false
			case <-ticker.C:
			}
		}
		ticker.Stop()
		s.mu.Lock()
		if s.hostConflict != nil && *s.hostConflict == owner {
			s.hostConflict = nil
		}
		s.hostConflictPreflight = false
		s.mu.Unlock()
		// 程序停止後重新查詢，避免多份殘留只清除其中一份。
	}
	return false
}
