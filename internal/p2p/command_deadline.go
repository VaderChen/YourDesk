package p2p

import (
	"context"
	"time"
)

const commandTimeout = 5 * time.Second

// 新版以剩餘時間傳遞預算；未公告能力的舊版仍只收到 expires。
func (r *CommandRequest) setTimeout(deadline time.Time, relative bool) error {
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return context.DeadlineExceeded
	}
	if relative {
		ms := (min(remaining, commandTimeout) + time.Millisecond - 1) / time.Millisecond
		value := int64(ms)
		r.TimeoutMS = &value
	}
	return nil
}

// 本機期限保留單調時鐘；不得將對端 Unix 時間當成本機的到期時間。
// 舊版維持原有期限檢查，不默默重新執行已逾時或結果未知的操作。
func commandDeadline(r CommandRequest, now time.Time) (time.Time, string, string) {
	if len(r.Params) > 8192 || len(r.Method) > 64 || r.Expires <= 0 {
		return time.Time{}, "invalid_request", "指令格式無效"
	}
	if r.TimeoutMS != nil {
		if *r.TimeoutMS <= 0 || *r.TimeoutMS > commandTimeout.Milliseconds() {
			return time.Time{}, "invalid_request", "指令逾時預算無效"
		}
		return now.Add(time.Duration(*r.TimeoutMS) * time.Millisecond), "", ""
	}
	if r.Expires > now.Add(30*time.Second).UnixMilli() {
		return time.Time{}, "invalid_request", "指令期限超出範圍，請同步兩端系統時間，或將兩端 YourDesk 更新至支援相對逾時的版本"
	}
	if r.Expires <= now.UnixMilli() {
		return time.Time{}, "expired", "指令已逾時，請檢查連線及兩端系統時間"
	}
	return now.Add(time.Duration(r.Expires-now.UnixMilli()) * time.Millisecond), "", ""
}
