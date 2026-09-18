//go:build windows

package main

import (
	"golang.org/x/sys/windows"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"
	"yourdesk/internal/rawkey"
)

var rawOnce sync.Once
var rawEnabled, rawReady atomic.Bool
var rawShortcutGuard atomic.Bool

func nativeSystemShortcutGuard(enabled bool) { rawShortcutGuard.Store(enabled) }

var rawMu sync.Mutex
var rawQueue []rawkey.Event
var rawHookCallback uintptr
var keyboardDLL = windows.NewLazySystemDLL("user32.dll")

func rawKeyboardAvailable() bool { startRawHook(); return rawReady.Load() }
func startRawHook() {
	rawOnce.Do(func() {
		go func() {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			next := keyboardDLL.NewProc("CallNextHookEx")
			foreground := keyboardDLL.NewProc("GetForegroundWindow")
			process := keyboardDLL.NewProc("GetWindowThreadProcessId")
			var held [512]bool
			wasCapturing := false
			asyncState := keyboardDLL.NewProc("GetAsyncKeyState")
			getKeyState := keyboardDLL.NewProc("GetKeyState")
			capsState, _, _ := getKeyState.Call(0x14)
			caps := capsState&1 != 0
			numState, _, _ := getKeyState.Call(0x90)
			num := numState&1 != 0
			rawHookCallback = windows.NewCallback(func(code int32, wparam, lparam uintptr) uintptr {
				pass := func() uintptr { v, _, _ := next.Call(0, uintptr(code), wparam, lparam); return v }
				if code < 0 {
					return pass()
				}
				// 原生對話框建立及放鍵等待期間，也不能讓重複快捷鍵落到本機。
				if rawShortcutGuard.Load() {
					hwnd, _, _ := foreground.Call()
					var pid uint32
					process.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
					e := (*struct {
						VK, Scan, Flags, Time uint32
						Extra                 uintptr
					})(unsafe.Pointer(lparam))
					ctrl, _, _ := asyncState.Call(0x11)
					shift, _, _ := asyncState.Call(0x10)
					if pid == uint32(os.Getpid()) && e.Flags&0x10 == 0 &&
						(e.VK == 0x73 && e.Flags&0x20 != 0 || e.VK == 0x57 && ctrl&0x8000 != 0 || e.VK == 0x1b && ctrl&0x8000 != 0 && shift&0x8000 != 0) {
						return 1
					}
				}
				if !rawEnabled.Load() || nativeTitlebarPopupOpen() {
					clear(held[:])
					wasCapturing = false
					capsState, _, _ = getKeyState.Call(0x14)
					caps = capsState&1 != 0
					numState, _, _ = getKeyState.Call(0x90)
					num = numState&1 != 0
					return pass()
				}
				hwnd, _, _ := foreground.Call()
				var pid uint32
				process.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
				if pid != uint32(os.Getpid()) {
					wasCapturing = false
					return pass()
				}
				if !wasCapturing {
					clear(held[:])
					for _, pair := range [][2]int{{0x2a, 0xa0}, {0x36, 0xa1}, {0x1d, 0xa2}, {0x11d, 0xa3}, {0x38, 0xa4}, {0x138, 0xa5}, {0x15b, 0x5b}, {0x15c, 0x5c}} {
						value, _, _ := asyncState.Call(uintptr(pair[1]))
						held[pair[0]] = value&0x8000 != 0
					}
					capsState, _, _ = getKeyState.Call(0x14)
					caps = capsState&1 != 0
					numState, _, _ = getKeyState.Call(0x90)
					num = numState&1 != 0
					wasCapturing = true
				}
				event := (*struct {
					VK, Scan, Flags, Time uint32
					Extra                 uintptr
				})(unsafe.Pointer(lparam))
				if event.Flags&0x10 != 0 {
					return pass()
				} // 不回傳遠端注入事件，避免同機連線迴圈。
				scan := int(event.Scan & 255)
				if event.Flags&1 != 0 {
					scan |= 256
				}
				// Num Lock 與 Pause 共用低位掃描碼，依原始 VK 補齊 E0。
				if event.VK == 0x90 {
					scan = 0x145
				}
				down := event.Flags&0x80 == 0
				repeat := down && held[scan]
				held[scan] = down
				if event.VK == 0x14 && down && !repeat {
					caps = !caps
				}
				if event.VK == 0x90 && down && !repeat {
					num = !num
				}
				var mods uint64 = 128 // Num Lock 狀態有效
				if num {
					mods |= 64
				}
				if caps {
					mods |= 16
				}
				if held[0x2a] || held[0x36] {
					mods |= 1
				}
				if held[0x1d] || held[0x11d] {
					mods |= 2
				}
				if held[0x38] || held[0x138] {
					mods |= 4
				}
				if held[0x15b] || held[0x15c] {
					mods |= 8
				}
				rawMu.Lock()
				if len(rawQueue) >= 1024 {
					rawQueue = []rawkey.Event{{Reset: true}}
				}
				rawQueue = append(rawQueue, rawkey.Event{Platform: "windows", Code: scan, Modifiers: mods, NativeFlags: uint64(event.Flags), Down: down, Repeat: repeat})
				rawMu.Unlock()
				return 1
			})
			hook, _, _ := keyboardDLL.NewProc("SetWindowsHookExW").Call(13, rawHookCallback, 0, 0)
			if hook == 0 {
				return
			}
			defer keyboardDLL.NewProc("UnhookWindowsHookEx").Call(hook)
			rawReady.Store(true)
			defer rawReady.Store(false)
			var msg struct {
				HWND           uintptr
				Message        uint32
				WParam, LParam uintptr
				Time           uint32
				X, Y           int32
				Private        uint32
			}
			for {
				v, _, _ := keyboardDLL.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
				if int32(v) <= 0 {
					return
				}
			}
		}()
	})
}
func captureRawKeys(enabled bool) []rawkey.Event {
	startRawHook()
	rawEnabled.Store(enabled)
	rawMu.Lock()
	defer rawMu.Unlock()
	result := rawQueue
	rawQueue = nil
	return result
}

func nativeSystemShortcutReleased(key string, mods uint64) bool {
	keys := []uintptr{map[string]uintptr{"q": 0x51, "w": 0x57, "f4": 0x73, "escape": 0x1b, "delete": 0x2e}[key]}
	for _, m := range []struct {
		bit uint64
		vk  uintptr
	}{{1, 0x10}, {2, 0x11}, {4, 0x12}, {8, 0x5b}, {8, 0x5c}} {
		if mods&m.bit != 0 {
			keys = append(keys, m.vk)
		}
	}
	for _, vk := range keys {
		if vk != 0 {
			value, _, _ := keyboardDLL.NewProc("GetAsyncKeyState").Call(vk)
			if value&0x8000 != 0 {
				return false
			}
		}
	}
	return true
}
