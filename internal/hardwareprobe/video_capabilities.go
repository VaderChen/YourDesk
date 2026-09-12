package hardwareprobe

import (
	"yourdesk/internal/optimization"
	"yourdesk/internal/video"
)

// CachedVideoCapabilities 避免主畫面在 detector 未完成前再建立同一組編解碼器。
// 失敗／取消後仍使用既有背景回退；讀取 API 從不等待原生探測。
func CachedVideoCapabilities() []video.IntraCapability {
	s := Snapshot()
	if s.Status == "not-started" || s.Status == "running" {
		return nil
	}
	if optimization.Snapshot().Version == 0 {
		if p := policyFromState(s); p != nil {
			optimization.Apply(*p)
		}
	}
	return video.CachedIntraCapabilities()
}
