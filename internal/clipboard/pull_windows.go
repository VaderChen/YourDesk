//go:build windows && cgo

package clipboard

/*
#cgo CXXFLAGS: -std=c++17
#cgo LDFLAGS: -static -static-libstdc++ -static-libgcc -lole32 -lshell32 -luuid
#include <stdlib.h>
#include "pull_windows.h"
*/
import "C"
import (
	"fmt"
	"io"
	"sync"
	"unsafe"
)

var windowsOffers = struct {
	sync.RWMutex
	next   uintptr
	values map[uintptr]*pullOffer
}{values: map[uintptr]*pullOffer{}}

func nativePullSupported() bool { return true }
func nativePublishOffer(o *pullOffer) error {
	windowsOffers.Lock()
	windowsOffers.next++
	token := windowsOffers.next
	windowsOffers.values[token] = o
	windowsOffers.Unlock()
	cleanup := func() { windowsOffers.Lock(); delete(windowsOffers.values, token); windowsOffers.Unlock() }
	obj := C.yd_pull_create(C.uintptr_t(token), C.int(len(o.entries)))
	if obj == nil {
		cleanup()
		return fmt.Errorf("無法建立 Windows 檔案清單")
	}
	defer C.yd_pull_discard(obj)
	for i, e := range o.entries {
		name := C.CString(e.Name)
		directory := 0
		if e.Directory {
			directory = 1
		}
		ok := C.yd_pull_entry(obj, C.int(i), name, C.int64_t(e.Size), C.int(directory))
		C.free(unsafe.Pointer(name))
		if ok == 0 {
			cleanup()
			return fmt.Errorf("Windows 虛擬檔案路徑過長或無效：%s", e.Name)
		}
	}
	hr := C.yd_pull_publish(obj)
	if hr < 0 {
		cleanup()
		return fmt.Errorf("Windows 未接受延遲貼上清單：0x%08x", uint32(hr))
	}
	o.s.pull.Lock()
	o.s.pull.cleanup = append(o.s.pull.cleanup, pullLease{offer: o, close: cleanup})
	o.s.pull.Unlock()
	return nil
}

//export goYDPullRead
func goYDPullRead(token C.uintptr_t, index C.int, offset C.int64_t, buffer unsafe.Pointer, length C.int) C.int {
	if length < 0 || length > pullReadSize || buffer == nil {
		return -1
	}
	windowsOffers.RLock()
	o := windowsOffers.values[uintptr(token)]
	windowsOffers.RUnlock()
	if o == nil {
		return -1
	}
	n, err := o.read(int(index), int64(offset), unsafe.Slice((*byte)(buffer), int(length)))
	if err != nil && err != io.EOF {
		return -1
	}
	return C.int(n)
}
