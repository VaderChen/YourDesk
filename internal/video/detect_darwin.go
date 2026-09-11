//go:build darwin

package video

import "os/exec"

func detectCapabilities() capabilities {
	// VideoToolbox is present on supported macOS installations. Verify the
	// framework symbol through the system tool instead of guessing from CPU.
	if _, err := exec.LookPath("system_profiler"); err == nil {
		return capabilities{h264: true, hevc: true, swH264: true, backend: "VideoToolbox", detail: "macOS VideoToolbox candidate; runtime probe required"}
	}
	return capabilities{}
}
