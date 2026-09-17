//go:build !darwin || !cgo

package main

func showNativeCursor() {}

func pollNativePointer()                      {}
func nativePointer() (float64, float64, bool) { return 0, 0, false }
