//go:build !darwin || !cgo

package clientui

import "unsafe"

// WebView2／GTK 直接處理使用者點擊原生 file input。
func installFilesPicker(window unsafe.Pointer) (func(), error) { return func() {}, nil }
