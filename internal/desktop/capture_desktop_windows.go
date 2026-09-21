//go:build windows

package desktop

import (
	"fmt"
	"golang.org/x/sys/windows"
	"unsafe"
)

var captureUser32 = windows.NewLazySystemDLL("user32.dll")
var captureOpenDesktop = captureUser32.NewProc("OpenInputDesktop")
var captureSetDesktop = captureUser32.NewProc("SetThreadDesktop")
var captureCloseDesktop = captureUser32.NewProc("CloseDesktop")
var captureDesktopInfo = captureUser32.NewProc("GetUserObjectInformationW")
var captureGetDesktop = captureUser32.NewProc("GetThreadDesktop")

type captureDesktop struct {
	captureDesktopBinding
	original uintptr
}

func newCaptureDesktop() *captureDesktop {
	original, _, _ := captureGetDesktop.Call(uintptr(windows.GetCurrentThreadId()))
	b := &captureDesktop{original: original}
	b.release = func(h uintptr) { captureCloseDesktop.Call(h) }
	b.bind = func(h uintptr) error {
		ok, _, err := captureSetDesktop.Call(h)
		if ok == 0 {
			return fmt.Errorf("切換擷取桌面失敗：%w", err)
		}
		return nil
	}
	b.open = func() (uintptr, string, error) {
		h, _, err := captureOpenDesktop.Call(0, 0, 0x0001) // DESKTOP_READOBJECTS
		if h == 0 {
			return 0, "", fmt.Errorf("開啟擷取桌面失敗：%w", err)
		}
		var name [256]uint16
		var needed uint32
		ok, _, err := captureDesktopInfo.Call(h, 2, uintptr(unsafe.Pointer(&name[0])), unsafe.Sizeof(name), uintptr(unsafe.Pointer(&needed)))
		if ok == 0 {
			b.release(h)
			return 0, "", fmt.Errorf("讀取擷取桌面失敗：%w", err)
		}
		return h, windows.UTF16ToString(name[:]), nil
	}
	return b
}

func (b *captureDesktop) close() {
	if b.current != 0 && b.original != 0 {
		if b.bind(b.original) == nil {
			b.release(b.current)
			b.current = 0
		}
	}
}
