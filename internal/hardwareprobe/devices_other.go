//go:build !windows

package hardwareprobe

// macOS CGO 使用原生 inventory；其他尚未實作的平台保留未知。
func platformDeviceNames() (map[string]string, []map[string]string) { return nil, nil }
