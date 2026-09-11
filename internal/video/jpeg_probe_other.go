//go:build !darwin && !windows

package video

// VA-API/V4L2 JPEG support is driver-specific and must process a test frame;
// command presence alone is deliberately not treated as hardware support.
func init() {
	RegisterHardwareJPEGProbe(func() JPEGCapabilities {
		return JPEGCapabilities{Backend: "none", Detail: "未找到通過實測的 VA-API/V4L2/NVJPEG backend", Probed: true}
	})
}
