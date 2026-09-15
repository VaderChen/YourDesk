package clipboard

import "yourdesk/internal/authlog"

// 只記錄階段與計數，不記錄剪貼簿內容、檔名或本機路徑。
// 沿用封包分析的有界背景佇列，不在剪貼簿處理路徑寫入磁碟。
func traceClipboard(stage, offer string, fields map[string]any) {
	if !authlog.IsEnabled() {
		return
	}
	if fields == nil {
		fields = make(map[string]any)
	}
	fields["stage"] = stage
	if offer != "" {
		fields["offer"] = offer
	}
	authlog.Event("clipboard", fields)
}
