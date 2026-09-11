//go:build darwin && cgo

package clientui

/*
#cgo LDFLAGS: -framework Cocoa
#include <stdint.h>
#include <stdlib.h>
void yd_set_app_icon(void *tray, const void *data, size_t length);
void *yd_tray_install(void *window, uintptr_t handle);
void yd_tray_remove(void *tray);
void yd_connection_notice(void *window);
void yd_update_window(void *window);
void yd_transfer_window(void *window);
void yd_close_interface(void *window);
void yd_incoming_tray(void *window, int connected);
void yd_mcp_tray(void *window, int count, int notify, int visible);
*/
import "C"

import (
	"errors"
	"runtime/cgo"
	"unsafe"
)

//export ydTrayQuit
func ydTrayQuit(handle C.uintptr_t) { cgo.Handle(handle).Value().(trayCallbacks).quit() }

//export ydTrayShowMCP
func ydTrayShowMCP(handle C.uintptr_t) { cgo.Handle(handle).Value().(trayCallbacks).showMCP() }

func installTray(window unsafe.Pointer, quit func(), showMCP func(), disconnectIncoming func()) (func(), error) {
	handle := cgo.NewHandle(trayCallbacks{quit: quit, showMCP: showMCP, disconnectIncoming: disconnectIncoming})
	tray := C.yd_tray_install(window, C.uintptr_t(handle))
	if tray == nil {
		handle.Delete()
		return nil, errors.New("無法建立 YourDesk 狀態列選單")
	}
	icon, err := assets.ReadFile("web/app-icon.png")
	if err != nil {
		C.yd_tray_remove(tray)
		handle.Delete()
		return nil, err
	}
	data := C.CBytes(icon)
	C.yd_set_app_icon(tray, data, C.size_t(len(icon)))
	C.free(data)
	return func() { C.yd_tray_remove(tray); handle.Delete() }, nil
}

func showConnectionNotice(window unsafe.Pointer) { C.yd_connection_notice(window) }

func showUpdateWindow(window unsafe.Pointer) { C.yd_update_window(window) }

func showTransferWindow(window unsafe.Pointer) { C.yd_transfer_window(window) }

func closeInterfaceWindow(window unsafe.Pointer) { C.yd_close_interface(window) }

func updateMCPTray(window unsafe.Pointer, count int, notify bool, visible bool) {
	n := 0
	if notify {
		n = 1
	}
	v := 0
	if visible {
		v = 1
	}
	C.yd_mcp_tray(window, C.int(count), C.int(n), C.int(v))
}

//export ydTrayDisconnectIncoming
func ydTrayDisconnectIncoming(handle C.uintptr_t) {
	cgo.Handle(handle).Value().(trayCallbacks).disconnectIncoming()
}
func updateIncomingTray(window unsafe.Pointer, connected bool) {
	n := 0
	if connected {
		n = 1
	}
	C.yd_incoming_tray(window, C.int(n))
}
