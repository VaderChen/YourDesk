//go:build !darwin || !cgo

package video

func newHardwareJPEGEncoder() (JPEGEncoder, error) { return nil, ErrHardwareJPEGUnavailable }
func newHardwareJPEGDecoder() (JPEGDecoder, error) { return nil, ErrHardwareJPEGUnavailable }
