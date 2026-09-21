//go:build !cgo || (!darwin && !windows)

package clientui

import (
	"errors"
	"unsafe"
)

func nativeInstallFileDrag(window unsafe.Pointer) (func([]string) error, func(), error) {
	return nil, nil, errors.New("此平台未提供系統檔案拖出；請使用下載按鈕")
}
