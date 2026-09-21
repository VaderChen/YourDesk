//go:build !cgo || (!darwin && !windows)

package clientui

import (
	"errors"
	"unsafe"
)

func installFilesClose(window unsafe.Pointer, request func()) (func(), error) {
	return nil, errors.New("此平台未提供檔案視窗關閉控制")
}
