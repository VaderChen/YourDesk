// Package hostguard 管理 Host 的本機唯一性與父程序生命週期。
package hostguard

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

var ErrOccupied = errors.New("此裝置已有 Host 執行，等待原程序結束")

// 鎖不依執行檔位置或版本；同一帳號、同一裝置 ID 共用。
func Acquire(ctx context.Context, room string, occupied func()) (func(), error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	dir = filepath.Join(dir, "YourDesk")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, fmt.Sprintf("host-%x.lock", sha256.Sum256([]byte(room))))
	notified := false
	for {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		release, err := tryLock(path)
		if err == nil {
			clearOwner := publishOwner(path)
			return func() { clearOwner(); release() }, nil
		}
		if !errors.Is(err, ErrOccupied) {
			return nil, err
		}
		if !notified {
			if occupied != nil {
				occupied()
			}
			notified = true
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
