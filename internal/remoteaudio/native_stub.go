//go:build !cgo || (!darwin && !windows)

package remoteaudio

import "errors"

func platformSupported() bool { return false }
func openNative(int, int, int, int) (device, error) {
	return nil, errors.New("此平台尚未支援遠端聲音")
}
