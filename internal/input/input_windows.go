//go:build windows

package input

import (
	"encoding/binary"
	"fmt"
	"unsafe"
	"yourdesk/internal/rawkey"

	"golang.org/x/sys/windows"
)

func EnsurePermissions() error { return nil }

type input struct {
	typ  uint32
	_    uint32
	data [32]byte
}

const (
	inputMouse      = 0
	inputKeyboard   = 1
	mouseMove       = 1
	mouseLeftDown   = 2
	mouseLeftUp     = 4
	mouseRightDown  = 8
	mouseRightUp    = 16
	mouseMiddleDown = 32
	mouseMiddleUp   = 64
	mouseWheel      = 0x0800
	keyUp           = 2
)

var sendInput = windows.NewLazySystemDLL("user32.dll").NewProc("SendInput")

func (n Native) Move(x, y float64) error {
	x, y = n.virtualPoint(x, y)
	var d [32]byte
	binary.LittleEndian.PutUint32(d[0:4], uint32(int32(x*65535)))
	binary.LittleEndian.PutUint32(d[4:8], uint32(int32(y*65535)))
	binary.LittleEndian.PutUint32(d[12:16], mouseMove|0x8000|0x4000)
	return send(input{typ: inputMouse, data: d})
}
func (Native) Button(button int, down bool) error {
	f := uint32(mouseLeftDown)
	if button == 2 {
		f = mouseRightDown
	} else if button == 3 {
		f = mouseMiddleDown
	}
	if !down {
		f <<= 1
	}
	var d [32]byte
	binary.LittleEndian.PutUint32(d[12:16], f)
	return send(input{typ: inputMouse, data: d})
}
func (n Native) ButtonAt(button int, down bool, x, y float64) error {
	x, y = n.virtualPoint(x, y)
	f := uint32(mouseLeftDown)
	if button == 2 {
		f = mouseRightDown
	} else if button == 3 {
		f = mouseMiddleDown
	}
	if !down {
		f <<= 1
	}
	var d [32]byte
	binary.LittleEndian.PutUint32(d[0:4], uint32(int32(x*65535)))
	binary.LittleEndian.PutUint32(d[4:8], uint32(int32(y*65535)))
	binary.LittleEndian.PutUint32(d[12:16], f|0x8000|0x4000|mouseMove)
	return send(input{typ: inputMouse, data: d})
}
func (Native) Key(key string, down bool) error {
	v, ok := keycodes[key]
	if !ok {
		return fmt.Errorf("不支援按鍵 %q", key)
	}
	f := uint32(0)
	if !down {
		f = keyUp
	}
	var d [32]byte
	binary.LittleEndian.PutUint16(d[0:2], uint16(v))
	binary.LittleEndian.PutUint32(d[4:8], f)
	return send(input{typ: inputKeyboard, data: d})
}
func (Native) Wheel(delta float64) error {
	var d [32]byte
	binary.LittleEndian.PutUint32(d[8:12], uint32(int32(delta*120)))
	binary.LittleEndian.PutUint32(d[12:16], mouseWheel)
	return send(input{typ: inputMouse, data: d})
}
func send(i input) error {
	r, _, e := sendInput.Call(1, uintptr(unsafe.Pointer(&i)), unsafe.Sizeof(i))
	_ = r
	if r != 1 {
		return e
	}
	return nil
}

var keycodes = map[string]int{"a": 0x41, "b": 0x42, "c": 0x43, "d": 0x44, "e": 0x45, "f": 0x46, "g": 0x47, "h": 0x48, "i": 0x49, "j": 0x4a, "k": 0x4b, "l": 0x4c, "m": 0x4d, "n": 0x4e, "o": 0x4f, "p": 0x50, "q": 0x51, "r": 0x52, "s": 0x53, "t": 0x54, "u": 0x55, "v": 0x56, "w": 0x57, "x": 0x58, "y": 0x59, "z": 0x5a, "0": 0x30, "1": 0x31, "2": 0x32, "3": 0x33, "4": 0x34, "5": 0x35, "6": 0x36, "7": 0x37, "8": 0x38, "9": 0x39, "enter": 0x0d, "escape": 0x1b, "space": 0x20, "backspace": 0x08, "tab": 0x09, "delete": 0x2e, "arrowup": 0x26, "arrowdown": 0x28, "arrowleft": 0x25, "arrowright": 0x27, "home": 0x24, "end": 0x23, "pageup": 0x21, "pagedown": 0x22, "shift": 0x10, "control": 0x11, "alt": 0x12, "meta": 0x5b, "f1": 0x70, "f2": 0x71, "f3": 0x72, "f4": 0x73, "f5": 0x74, "f6": 0x75, "f7": 0x76, "f8": 0x77, "f9": 0x78, "f10": 0x79, "f11": 0x7a, "f12": 0x7b}

// 絕對座標以整個虛擬桌面正規化，搭配 MOUSEEVENTF_VIRTUALDESK。
func (n Native) virtualPoint(x, y float64) (float64, float64) {
	x, y = n.point(x, y)
	metric := func(index uintptr) float64 { v, _, _ := getSystemMetrics.Call(index); return float64(int32(v)) }
	return (x - metric(76)) / max(1, metric(78)-1), (y - metric(77)) / max(1, metric(79)-1)
}

var getSystemMetrics = windows.NewLazySystemDLL("user32.dll").NewProc("GetSystemMetrics")

var rawLockState = windows.NewLazySystemDLL("user32.dll").NewProc("GetKeyState")

func (Native) RawKey(e rawkey.Event) error {
	code, err := rawkey.TranslateEvent(e, "windows")
	if err != nil {
		return err
	}
	name := rawkey.Name("windows", code)
	// 鎖定鍵依來源的狀態快照決定是否切換，避免兩端初始狀態相反。
	lockVK, lockBit := uintptr(0), uint64(0)
	if name == "capslock" {
		lockVK, lockBit = 0x14, 16
	}
	if name == "numlock" && e.Modifiers&128 != 0 {
		lockVK, lockBit = 0x90, 64
	}
	if lockVK != 0 {
		if !e.Down || e.Repeat {
			return nil
		}
		state, _, _ := rawLockState.Call(lockVK)
		if (state&1 != 0) == (e.Modifiers&lockBit != 0) {
			return nil
		}
		// 以成對事件切換；不把鎖定鍵留在 held 狀態。
		if err := sendRawScan(code, true); err != nil {
			return err
		}
		return sendRawScan(code, false)
	}
	return sendRawScan(code, e.Down)
}

func sendRawScan(code int, down bool) error {
	var d [32]byte
	binary.LittleEndian.PutUint16(d[2:4], uint16(code&255))
	flags := uint32(0x0008) // KEYEVENTF_SCANCODE，交由遠端鍵盤配置與輸入法解讀。
	if code&256 != 0 {
		flags |= 1
	}
	if !down {
		flags |= keyUp
	}
	if rawkey.Name("windows", code) == "pause" {
		binary.LittleEndian.PutUint16(d[0:2], 0x13)
		binary.LittleEndian.PutUint16(d[2:4], 0)
		flags = 0
		if !down {
			flags = keyUp
		}
	}
	binary.LittleEndian.PutUint32(d[4:8], flags)
	return send(input{typ: inputKeyboard, data: d})
}
