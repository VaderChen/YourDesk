//go:build darwin && cgo

package main

/*
#include <stdint.h>
void yd_keyboard_enabled(int enabled);
int yd_keyboard_pop(int *code,uint64_t *mods,uint64_t *flags,int *down,int *repeat,int *reset);
int yd_shortcut_released(int code, uint64_t mods);
void yd_keyboard_shortcut_guard(int value);
*/
import "C"
import "yourdesk/internal/rawkey"

func captureRawKeys(enabled bool) []rawkey.Event {
	value := 0
	if enabled {
		value = 1
	}
	C.yd_keyboard_enabled(C.int(value))
	var events []rawkey.Event
	for {
		var code, down, repeat, reset C.int
		var mods, flags C.uint64_t
		if C.yd_keyboard_pop(&code, &mods, &flags, &down, &repeat, &reset) == 0 {
			break
		}
		events = append(events, rawkey.Event{Platform: "darwin", Code: int(code), Modifiers: uint64(mods), NativeFlags: uint64(flags), Down: down != 0, Repeat: repeat != 0, Reset: reset != 0})
	}
	return events
}
func rawKeyboardAvailable() bool { return true }
func nativeSystemShortcutGuard(enabled bool) {
	value := C.int(0)
	if enabled {
		value = 1
	}
	C.yd_keyboard_shortcut_guard(value)
}

func nativeSystemShortcutReleased(key string, mods uint64) bool {
	code, ok := rawkey.Code("darwin", key)
	return ok && C.yd_shortcut_released(C.int(code), C.uint64_t(mods)) != 0
}
