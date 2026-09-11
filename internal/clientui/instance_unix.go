//go:build darwin || linux

package clientui

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
)

// 鎖由作業系統隨程序結束釋放，不以容易殘留的 PID 檔判斷。
func lockInstance(path string) (func(), error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		file.Close()
		return nil, fmt.Errorf("YourDesk 已在執行，請從 Tray 開啟現有介面：%w", err)
	}
	return func() { file.Close() }, nil
}
