//go:build !cgo || !turbojpeg

package video

const preferredSoftwareJPEGBackend = "Go image/jpeg"

func newTurboJPEGEncoder() JPEGEncoder { return nil }
func newTurboJPEGDecoder() JPEGDecoder { return nil }
