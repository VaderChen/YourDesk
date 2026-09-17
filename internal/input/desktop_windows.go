//go:build windows

package input

import (
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var inputUser32 = windows.NewLazySystemDLL("user32.dll")
var openInputDesktop = inputUser32.NewProc("OpenInputDesktop")
var setThreadDesktop = inputUser32.NewProc("SetThreadDesktop")
var closeDesktop = inputUser32.NewProc("CloseDesktop")
var getDesktopInformation = inputUser32.NewProc("GetUserObjectInformationW")

type desktopInputRequest struct {
	inject func() error
	done   chan error
}

var desktopInputOnce sync.Once
var desktopInputQueue chan desktopInputRequest

// SetThreadDesktop 與 SendInput 必須在同一個、不建立視窗或掛鉤的
// OS 執行緒上操作。此工作者為程序共用，不能讓 Go 排程切換執行緒。
func onInputDesktop(inject func() error) error {
	desktopInputOnce.Do(func() {
		desktopInputQueue = make(chan desktopInputRequest)
		go func() {
			runtime.LockOSThread()
			binding := desktopBinding{
				open: func() (inputDesktop, error) {
					// READOBJECTS | WRITEOBJECTS；不切換使用者可見桌面、不變更權限。
					h, _, err := openInputDesktop.Call(0, 0, 0x0001|0x0080)
					if h == 0 {
						return inputDesktop{}, inputAPIError("OpenInputDesktop", err)
					}
					var name [256]uint16
					var needed uint32
					ok, _, err := getDesktopInformation.Call(h, 2, uintptr(unsafe.Pointer(&name[0])), unsafe.Sizeof(name), uintptr(unsafe.Pointer(&needed)))
					if ok == 0 {
						closeDesktop.Call(h)
						return inputDesktop{}, inputAPIError("GetUserObjectInformationW", err)
					}
					return inputDesktop{handle: h, name: windows.UTF16ToString(name[:])}, nil
				},
				bind: func(h uintptr) error {
					ok, _, err := setThreadDesktop.Call(h)
					if ok == 0 {
						return inputAPIError("SetThreadDesktop", err)
					}
					return nil
				},
				close: func(h uintptr) { closeDesktop.Call(h) },
			}
			for request := range desktopInputQueue {
				request.done <- binding.run(request.inject)
			}
		}()
	})
	request := desktopInputRequest{inject: inject, done: make(chan error, 1)}
	desktopInputQueue <- request
	return <-request.done
}

func inputAPIError(name string, err error) error {
	if err == nil || err == syscall.Errno(0) {
		return fmt.Errorf("%s 未完成，Windows 未提供錯誤碼", name)
	}
	return fmt.Errorf("%s：%w", name, err)
}
