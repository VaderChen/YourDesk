package main

import "yourdesk/internal/video"

func sourceEncodingMode(codec byte, reported string) string {
	if reported == "hardware" || reported == "software" {
		return reported
	}
	// 現行 wire H.264／HEVC 僅由已驗證的硬體編碼器產生；軟體備援會改送 JPEG。
	// 舊 Client 未提供 video-status，或控制訊息晚於影格時，可依此協定保證判定。
	if codec == byte(video.WireH264) || codec == byte(video.WireHEVC) {
		return "hardware"
	}
	// JPEG 同時有軟體／硬體實作，缺少來源回報時不能從接收端硬體能力推測。
	return "unknown"
}
