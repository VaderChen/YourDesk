//go:build darwin && !cgo

package video

func init() {
	RegisterHardwareJPEGProbe(func() JPEGCapabilities {
		return JPEGCapabilities{Backend: "none", Detail: "CGO 關閉，無法探測 VideoToolbox JPEG", Probed: true}
	})
}
