//go:build windows

package video

// WIC does not expose a trustworthy hardware flag. Media Foundation/NVJPEG
// must successfully instantiate and process a test frame before this probe is
// allowed to return true. Until that backend is linked, fail closed.
func init() {
	RegisterHardwareJPEGProbe(func() JPEGCapabilities {
		return JPEGCapabilities{Backend: "none", Detail: "Windows 未找到通過實測的硬體 MJPEG MFT/NVJPEG backend", Probed: true}
	})
}
