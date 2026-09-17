// Package reconnect 提供有上限且可取消的背景重連。
package reconnect

import (
	"context"
	"time"
)

const Attempts = 3
const AttemptTimeout = 30 * time.Second

// Run 的 attempt 必須遵守 context；回傳 nil 代表已收到新連線的畫面。
func Run(ctx context.Context, attempt func(context.Context) error) error {
	var last error
	for i := 0; i < Attempts; i++ {
		delay := time.Duration(2+i*3) * time.Second
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		request, cancel := context.WithTimeout(ctx, AttemptTimeout)
		last = attempt(request)
		cancel()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if last == nil {
			return nil
		}
	}
	return last
}
