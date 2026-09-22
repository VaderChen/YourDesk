//go:build !cgo || !opus

package remoteaudio

import "errors"

const opusAvailable = false

func openOpus(int, int, int) (device, error) {
	return nil, errors.New("此建置未包含 Opus，請使用完整桌面版建置")
}
