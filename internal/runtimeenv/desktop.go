// Package runtimeenv 在啟動時偵測目前登入工作階段，不將無桌面模式當成另一套程式。
package runtimeenv

import (
	"os"
	"runtime"
)

// Linux 的無圖形工作階段不初始化 WebView、擷取或剪貼簿。
// Windows 登入畫面及 macOS 權限問題沿用各平台機制，不視為無桌面 OS。
func Headless() bool {
	return headlessSession(runtime.GOOS, os.Getenv("DISPLAY"), os.Getenv("WAYLAND_DISPLAY"))
}

func headlessSession(system, display, wayland string) bool {
	return system == "linux" && display == "" && wayland == ""
}
