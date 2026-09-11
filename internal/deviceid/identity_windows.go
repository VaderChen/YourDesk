//go:build windows

package deviceid

import (
	"context"
	"errors"
	"strings"
	"time"

	"golang.org/x/sys/windows/registry"
	"yourdesk/internal/childprocess"
)

// Windows 的 ProcessorId 是 CPU 特徵，不能當作每台電腦的唯一識別。
// 固定讀取 64-bit registry view，讓不同架構、帳號與 Host 子程序一致。
func platformIdentity() (Identity, error) {
	if raw, err := machineGUID(); err == nil {
		if normalized, ok := normalizeWindowsGUID(raw); ok {
			return Identity{UID: encode(SourceWindowsMachine, normalized), Source: SourceWindowsMachine}, nil
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := childprocess.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "(Get-CimInstance Win32_ComputerSystemProduct -ErrorAction Stop | Select-Object -First 1 -ExpandProperty UUID)").Output()
	if err == nil {
		if normalized, ok := normalizeWindowsGUID(string(out)); ok {
			return Identity{UID: encode(SourceSystemUUID, normalized), Source: SourceSystemUUID}, nil
		}
	}
	// 無法辨識時明確失敗，不退回可能撞號的 CPU 資訊或共用預設值。
	return Identity{}, errors.New("無法取得有效的 Windows MachineGuid 或系統 UUID")
}

func machineGUID() (string, error) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Cryptography`, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return "", err
	}
	defer key.Close()
	value, _, err := key.GetStringValue("MachineGuid")
	return value, err
}

func normalizeWindowsGUID(raw string) (string, bool) {
	value := strings.ToUpper(strings.TrimSpace(raw))
	if len(value) == 38 && value[0] == '{' && value[37] == '}' {
		value = value[1:37]
	}
	if len(value) != 36 {
		return "", false
	}
	allZero, allF := true, true
	for i, c := range value {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return "", false
			}
			continue
		}
		if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'F') {
			return "", false
		}
		allZero = allZero && c == '0'
		allF = allF && c == 'F'
	}
	return value, !allZero && !allF
}
