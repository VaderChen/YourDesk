//go:build cgo && (darwin || windows)

package clientui

/*
#cgo darwin LDFLAGS: -framework Cocoa
#cgo windows LDFLAGS: -lcomctl32
#include <stdint.h>
void *yd_files_close_install(void *window, uintptr_t callback);
void yd_files_close_remove(void *guard);
*/
import "C"

import (
	"errors"
	"runtime/cgo"
	"unsafe"
)

// Installation, callbacks and removal are confined to the native UI thread.
func installFilesClose(window unsafe.Pointer, request func()) (func(), error) {
	if window == nil || request == nil {
		return nil, errors.New("無法建立檔案視窗關閉控制")
	}
	handle := cgo.NewHandle(request)
	guard := C.yd_files_close_install(window, C.uintptr_t(handle))
	if guard == nil {
		handle.Delete()
		return nil, errors.New("無法建立檔案視窗關閉控制")
	}
	removed := false
	return func() {
		if !removed {
			removed = true
			C.yd_files_close_remove(guard)
			handle.Delete()
		}
	}, nil
}

//export ydFilesRequestClose
func ydFilesRequestClose(handle C.uintptr_t) {
	cgo.Handle(handle).Value().(func())()
}
