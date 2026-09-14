// Package buildinfo 提供不依賴桌面介面的共用建置資訊。
package buildinfo

import "os"

// Version 由建置腳本注入；開發建置未注入時沿用執行檔修改時間。
var Version string

func Current() string {
	if Version != "" {
		return Version
	}
	if path, err := os.Executable(); err == nil {
		if info, err := os.Stat(path); err == nil {
			return "1." + info.ModTime().Format("06.0102") + " build " + info.ModTime().Format("1504")
		}
	}
	return "未知版本"
}
