package clientui

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Version 由建置腳本注入，未注入時採用執行檔建置時間。
var Version string

// ApplicationVersion 與管理介面使用相同版號，供遠端能力公告使用。
func ApplicationVersion() string { return currentVersion() }

func currentVersion() string {
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

// 正式發行標籤接受 v1.YY.MMDD-build.HHmm 或畫面顯示格式。
var releaseVersion = regexp.MustCompile(`^v?(\d+)\.(\d{2})\.(\d{4})(?: build |[-.]build[.-])(\d{4})$`)

func versionKey(value string) string {
	parts := releaseVersion.FindStringSubmatch(strings.TrimSpace(value))
	if parts == nil {
		return ""
	}
	return fmt.Sprintf("%08s%s%s%s", parts[1], parts[2], parts[3], parts[4])
}
