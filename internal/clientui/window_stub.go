//go:build !cgo || (!darwin && !windows && !linux)

package clientui

import (
	"context"
	"errors"
)

func windowAvailable() error {
	return errors.New("此執行檔未包含原生桌面 UI，請使用 CGO_ENABLED=1 在支援的平台重新編譯；命令列 Client 可照常使用")
}
func runWindow(context.Context, string, <-chan struct{}, <-chan struct{}, <-chan struct{}, <-chan struct{}, *server) error {
	return windowAvailable()
}
