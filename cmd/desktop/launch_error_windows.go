//go:build windows

package main

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os/exec"
	"unsafe"
)

func launchFailureText(err error) string {
	message := fmt.Sprintf("YourDesk 主介面啟動失敗：%v", err)
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		code := uint32(exit.ExitCode())
		message += fmt.Sprintf("\nWindows 結束碼：0x%08X", code)
		switch code {
		case 0xc0000135:
			message += "\n缺少程式所需的 DLL。請確認完整安裝，並檢查防毒隔離紀錄。"
		case 0xc000007b:
			message += "\nEXE 與 DLL 的架構或格式不相容，請使用相同架構的完整安裝包。"
		case 0xc0000139:
			message += "\nDLL 缺少所需函式，請確認 EXE 與 DLL 來自同一版本。"
		}
	}
	return message
}
func reportLaunchFailure(err error) {
	text, _ := windows.UTF16PtrFromString(launchFailureText(err))
	title, _ := windows.UTF16PtrFromString("YourDesk 啟動失敗")
	windows.NewLazySystemDLL("user32.dll").NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x10)
}
