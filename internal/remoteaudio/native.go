//go:build cgo && (darwin || windows)

package remoteaudio

/*
#cgo darwin CFLAGS: -Wno-deprecated-declarations
#cgo darwin LDFLAGS: -framework AudioToolbox -framework CoreAudio -framework ScreenCaptureKit -framework CoreMedia -framework Foundation
#cgo windows CXXFLAGS: -std=c++17
#cgo windows LDFLAGS: -lole32 -luuid -lmfplat -lmfuuid -lmf -lwmcodecdspuuid -lpropsys -lstdc++
#include "native.h"
*/
import "C"
import (
	"errors"
	"unsafe"
)

type nativeDevice struct {
	ptr unsafe.Pointer
	hw  bool
}

func platformSupported() bool { return true }
func openNative(mode, codec, bitrate, preference int) (device, error) {
	var hw C.int
	message := make([]byte, 512)
	p := C.yd_audio_open(C.int(mode), C.int(codec), C.int(bitrate), C.int(preference), &hw, (*C.char)(unsafe.Pointer(&message[0])), C.int(len(message)))
	if p == nil && preference == 1 {
		return openNative(mode, codec, bitrate, 0)
	}
	if p == nil {
		return nil, errors.New(C.GoString((*C.char)(unsafe.Pointer(&message[0]))))
	}
	return &nativeDevice{p, hw != 0}, nil
}
func (d *nativeDevice) hardware() bool { return d.hw }
func (d *nativeDevice) close() {
	if d.ptr != nil {
		C.yd_audio_close(d.ptr)
		d.ptr = nil
	}
}
func (d *nativeDevice) process(data []byte) ([]byte, error) {
	out := make([]byte, MaxPacket)
	var in unsafe.Pointer
	if len(data) > 0 {
		in = unsafe.Pointer(&data[0])
	}
	n := int(C.yd_audio_process(d.ptr, in, C.int(len(data)), unsafe.Pointer(&out[0]), C.int(len(out))))
	if n < 0 {
		return nil, nativeError(n)
	}
	if n > len(out) {
		return nil, errors.New("原生聲音資料超出容量")
	}
	return out[:n], nil
}
