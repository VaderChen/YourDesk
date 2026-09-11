//go:build linux && cgo

package clientui

import (
	"errors"
	"unsafe"
)

func installTray(unsafe.Pointer, func(), func(), func()) (func(), error) {
	return nil, errors.New("Linux 尚未提供常駐 Tray 介面；目前請使用 macOS 或 Windows Client UI")
}

func showConnectionNotice(unsafe.Pointer) {}

func showUpdateWindow(unsafe.Pointer) {}

func showTransferWindow(unsafe.Pointer) {}

func closeInterfaceWindow(unsafe.Pointer) {}

func updateMCPTray(unsafe.Pointer, int, bool, bool) {}

func updateIncomingTray(unsafe.Pointer, bool) {}
