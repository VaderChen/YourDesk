package clipboard

import (
	"context"
	"encoding/json"
	"fmt"
)

// 接收資料與系統剪貼簿發布分開。Finder 掛載或 writeObjects 可能同步要求
// 遠端內容；這時讀取回覆仍須持續送達，不能等待 nativePublishOffer 返回。
// 檔案讀取工作由固定數量 pullWorker 執行，其餘訊息保留原本的順序與有界緩衝。
func (s *Sync) routePackets(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case data := <-s.peer.ClipboardMessages():
			if s.handlePullData(data) {
				continue
			}
			if len(data) > 1 && data[0] == 0 {
				var m packet
				if json.Unmarshal(data[1:], &m) == nil {
					switch m.Type {
					case "pull-read", "pull-error":
						s.handlePull(ctx, m)
						continue
					case "ack":
						s.mu.Lock()
						ack := s.acks[m.ID]
						s.mu.Unlock()
						if ack != nil {
							var err error
							if m.Error != "" {
								err = fmt.Errorf("接收端：%s", m.Error)
							}
							select {
							case ack <- err:
							default:
							}
						}
						continue
					}
				}
			}
			select {
			case s.packets <- data:
			case <-ctx.Done():
				return
			}
		}
	}
}
