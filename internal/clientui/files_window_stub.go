//go:build !cgo || (!darwin && !windows && !linux)

package clientui

import (
	"context"
	"errors"
)

func RunFilesWindow(context.Context) error {
	return errors.New("此建置不支援原生檔案傳輸視窗")
}
