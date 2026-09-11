package clientui

import (
	"fmt"
	"golang.org/x/sys/windows"
	"os"
)

func lockInstance(path string) (func(), error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	overlapped := &windows.Overlapped{}
	if err = windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, overlapped); err != nil {
		file.Close()
		return nil, fmt.Errorf("YourDesk 已在執行，請從 Tray 開啟現有介面：%w", err)
	}
	return func() { file.Close() }, nil
}
