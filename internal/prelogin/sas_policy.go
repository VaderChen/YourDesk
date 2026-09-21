package prelogin

import "errors"

// 僅新增服務權限，保留原有的輔助使用應用程式權限。
func sasPolicyWithServices(current uint64) (uint32, error) {
	if current > 3 {
		return 0, errors.New("無法辨識目前 SAS 系統原則")
	}
	return uint32(current) | 1, nil
}
