//go:build !windows && !linux

package hardwareprobe

// macOS 原生偵測已提供 hw.memsize；其他尚未實作的平台保留未知。
func physicalMemoryBytes() any { return nil }
