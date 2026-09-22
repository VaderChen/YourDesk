//go:build cgo && darwin

package clientui

/*
#cgo LDFLAGS: -framework Cocoa -framework WebKit
void *yd_files_picker_install(void *window);
void yd_files_picker_remove(void *guard);
*/
import "C"

import (
	"errors"
	"unsafe"
)

func installFilesPicker(window unsafe.Pointer) (func(), error) {
	guard := C.yd_files_picker_install(window)
	if guard == nil {
		return nil, errors.New("無法建立原生選檔視窗")
	}
	return func() { C.yd_files_picker_remove(guard) }, nil
}
