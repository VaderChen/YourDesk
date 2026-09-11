//go:build !darwin && !windows

package video

import "os/exec"

func detectCapabilities() capabilities {
	if _, err := exec.LookPath("vainfo"); err == nil {
		return capabilities{h264: true, hevc: true, swH264: true, backend: "VA-API", detail: "VA-API candidate; runtime probe required"}
	}
	return capabilities{}
}
