//go:build darwin && cgo

package main

/*
#cgo LDFLAGS: -framework Cocoa
void yd_show_native_cursor(void);
void yd_poll_native_pointer(void);
int yd_native_pointer(double *x, double *y);
*/
import "C"

func showNativeCursor() { C.yd_show_native_cursor() }

func pollNativePointer() { C.yd_poll_native_pointer() }
func nativePointer() (float64, float64, bool) {
	var x, y C.double
	valid := C.yd_native_pointer(&x, &y) != 0
	return float64(x), float64(y), valid
}
