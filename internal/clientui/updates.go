package clientui

import (
	"fmt"
	"regexp"
	"strings"
	"yourdesk/internal/buildinfo"
)

// ApplicationVersion 與管理介面共用獨立的建置資訊。
func ApplicationVersion() string { return buildinfo.Current() }
func currentVersion() string     { return buildinfo.Current() }

// 正式發行標籤接受 v1.YY.MMDD-build.HHmm 或畫面顯示格式。
var releaseVersion = regexp.MustCompile(`^v?(\d+)\.(\d{2})\.(\d{4})(?: build |[-.]build[.-])(\d{4})$`)

func versionKey(value string) string {
	parts := releaseVersion.FindStringSubmatch(strings.TrimSpace(value))
	if parts == nil {
		return ""
	}
	return fmt.Sprintf("%08s%s%s%s", parts[1], parts[2], parts[3], parts[4])
}
