//go:build windows && cgo

package clientui

/*
#cgo CXXFLAGS: -std=c++17
#cgo LDFLAGS: -lole32 -lshell32 -luuid -lcomctl32 -lgdi32
#include <stdlib.h>
void *yd_file_drag_install(void *window);
int yd_file_drag_set(void *view, const char **paths, int count);
void yd_file_drag_remove(void *view);
*/
import "C"

import (
	"errors"
	"unsafe"
)

func nativeInstallFileDrag(window unsafe.Pointer) (func([]string) error, func(), error) {
	view := C.yd_file_drag_install(window)
	if view == nil {
		return nil, nil, errors.New("無法建立 Explorer 檔案拖曳區")
	}
	set := func(paths []string) error {
		values := make([]*C.char, len(paths))
		for i, path := range paths {
			values[i] = C.CString(path)
			defer C.free(unsafe.Pointer(values[i]))
		}
		var pointer **C.char
		if len(values) != 0 {
			pointer = &values[0]
		}
		if C.yd_file_drag_set(view, pointer, C.int(len(values))) == 0 {
			return errors.New("Explorer 檔案拖曳區已關閉或來源無效")
		}
		return nil
	}
	return set, func() { C.yd_file_drag_remove(view) }, nil
}
