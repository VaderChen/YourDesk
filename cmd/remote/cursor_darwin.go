//go:build darwin && cgo

package main

/*
#cgo LDFLAGS: -framework Cocoa
void yd_show_native_cursor(void);
*/
import "C"

func showNativeCursor() { C.yd_show_native_cursor() }
