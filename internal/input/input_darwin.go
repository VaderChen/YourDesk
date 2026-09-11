//go:build darwin

package input

/*
#cgo LDFLAGS: -framework CoreGraphics -framework ApplicationServices -framework IOKit -framework Cocoa
#include <CoreGraphics/CoreGraphics.h>
#include <ApplicationServices/ApplicationServices.h>
#include <IOKit/hidsystem/IOHIDLib.h>
#include <IOKit/hidsystem/IOHIDParameter.h>
static int yd_request_accessibility(int prompt) { const void *keys[]={kAXTrustedCheckOptionPrompt}; const void *values[]={prompt ? kCFBooleanTrue : kCFBooleanFalse}; CFDictionaryRef opts=CFDictionaryCreate(kCFAllocatorDefault,keys,values,1,&kCFCopyStringDictionaryKeyCallBacks,&kCFTypeDictionaryValueCallBacks); Boolean ok=AXIsProcessTrustedWithOptions(opts); CFRelease(opts); return ok ? 1 : 0; }
void yd_mouse_move(double x, double y);
void yd_mouse_button(double x, double y, int button, int down);
// CGEvent coordinates are logical screen points, not Retina backing pixels.
static void yd_move(double x,double y) { yd_mouse_move(x,y); }
static void yd_button(int b,int down) {
 CGEventRef src=CGEventCreate(NULL);if(!src)return;
 CGPoint p=CGEventGetLocation(src);CFRelease(src);
 yd_mouse_button(p.x,p.y,b,down);
}
static void yd_button_at(double x,double y,int b,int down) { yd_mouse_button(x,y,b,down); }
static void yd_key(int code,int down) { CGEventRef e=CGEventCreateKeyboardEvent(NULL,(CGKeyCode)code,down != 0); CGEventPost(kCGHIDEventTap,e); CFRelease(e); }
static int yd_modifier_lock(int selector,int on) {
 io_service_t service=IOServiceGetMatchingService(kIOMainPortDefault,IOServiceMatching("IOHIDSystem"));
 if(!service)return -1;
 io_connect_t connection=IO_OBJECT_NULL;
 kern_return_t result=IOServiceOpen(service,mach_task_self(),kIOHIDParamConnectType,&connection);
 IOObjectRelease(service);
 if(result!=KERN_SUCCESS)return result;
 result=IOHIDSetModifierLockState(connection,selector,on!=0);
 IOServiceClose(connection);return result;
}
static void yd_raw_key(int code,int down,unsigned long long mods,int repeat,int numeric,unsigned long long nativeFlags) {
 CGEventRef e=CGEventCreateKeyboardEvent(NULL,(CGKeyCode)code,down!=0);
 CGEventFlags flags=0;
 if(mods&1)flags|=kCGEventFlagMaskShift;
 if(mods&2)flags|=kCGEventFlagMaskControl;
 if(mods&4)flags|=kCGEventFlagMaskAlternate;
 if(mods&8)flags|=kCGEventFlagMaskCommand;
 if(mods&16)flags|=kCGEventFlagMaskAlphaShift;
 if(mods&32)flags|=kCGEventFlagMaskSecondaryFn;
 if(numeric)flags|=kCGEventFlagMaskNumericPad;
 if(code==57){
  CGEventSetType(e,kCGEventFlagsChanged);
  // 保留 Caps Lock 的實體狀態，讓系統輸入來源處理器辨識按下／放開。
  flags|=nativeFlags&(NX_ALPHASHIFT_STATELESS_MASK|NX_DEVICE_ALPHASHIFT_STATELESS_MASK);
 }
 CGEventSetFlags(e,flags);
 CGEventSetIntegerValueField(e,kCGEventSourceUserData,0x5944524b);
 CGEventSetIntegerValueField(e,kCGKeyboardEventAutorepeat,repeat);
 CGEventPost(kCGHIDEventTap,e);CFRelease(e);
}
static void yd_wheel(int delta) { CGEventRef e=CGEventCreateScrollWheelEvent(NULL,kCGScrollEventUnitLine,1,delta); CGEventPost(kCGHIDEventTap,e); CFRelease(e); }
*/
import "C"
import (
	"fmt"
	"strings"
	"sync/atomic"
	"yourdesk/internal/rawkey"
)

// 每個 Client 程序只提出一次授權提示，重連與 IP 直連共用此狀態。
// 後續只讀取權限；使用者授權後，下次重試即可繼續，不必重啟。
var accessibilityPrompted atomic.Bool

func EnsurePermissions() error {
	prompt := C.int(0)
	if !accessibilityPrompted.Swap(true) {
		prompt = 1
	}
	if C.yd_request_accessibility(prompt) == 0 {
		return fmt.Errorf("macOS 尚未允許輔助使用；請到 系統設定 > 隱私權與安全性 > 輔助使用，允許 YourDesk（命令列啟動則允許對應程式）；授權後會自動重試")
	}
	return nil
}

func (n Native) Move(x, y float64) error {
	x, y = n.point(x, y)
	C.yd_move(C.double(x), C.double(y))
	return nil
}
func (Native) Button(button int, down bool) error {
	d := 0
	if down {
		d = 1
	}
	C.yd_button(C.int(button), C.int(d))
	return nil
}
func (n Native) ButtonAt(button int, down bool, x, y float64) error {
	x, y = n.point(x, y)
	d := 0
	if down {
		d = 1
	}
	C.yd_button_at(C.double(x), C.double(y), C.int(button), C.int(d))
	return nil
}
func (Native) Key(key string, down bool) error {
	code, ok := keycodes[key]
	if !ok {
		return fmt.Errorf("不支援按鍵 %q", key)
	}
	d := 0
	if down {
		d = 1
	}
	C.yd_key(C.int(code), C.int(d))
	return nil
}
func (Native) Wheel(delta float64) error {
	if delta != 0 {
		C.yd_wheel(C.int(delta))
	}
	return nil
}

var keycodes = map[string]int{"a": 0, "b": 11, "c": 8, "d": 2, "e": 14, "f": 3, "g": 5, "h": 4, "i": 34, "j": 38, "k": 40, "l": 37, "m": 46, "n": 45, "o": 31, "p": 35, "q": 12, "r": 15, "s": 1, "t": 17, "u": 32, "v": 9, "w": 13, "x": 7, "y": 16, "z": 6, "0": 29, "1": 18, "2": 19, "3": 20, "4": 21, "5": 23, "6": 22, "7": 26, "8": 28, "9": 25, "enter": 36, "escape": 53, "space": 49, "backspace": 51, "tab": 48, "delete": 117, "arrowup": 126, "arrowdown": 125, "arrowleft": 123, "arrowright": 124, "home": 115, "end": 119, "pageup": 116, "pagedown": 121, "shift": 56, "control": 59, "alt": 58, "meta": 55, "f1": 122, "f2": 120, "f3": 99, "f4": 118, "f5": 96, "f6": 97, "f7": 98, "f8": 100, "f9": 101, "f10": 109, "f11": 103, "f12": 111}

func (Native) RawKey(e rawkey.Event) error {
	code, err := rawkey.TranslateEvent(e, "darwin")
	if err != nil {
		return err
	}
	down, repeat := 0, 0
	if e.Down {
		down = 1
	}
	if e.Repeat {
		repeat = 1
	}
	if code == 57 && e.Platform != "darwin" {
		if !e.Down {
			return nil
		}
		on := 0
		if e.Modifiers&16 != 0 {
			on = 1
		}
		if status := C.yd_modifier_lock(C.kIOHIDCapsLockState, C.int(on)); status != 0 {
			return fmt.Errorf("Caps Lock 狀態同步失敗：%d", status)
		}
	}
	if code == 71 && e.Modifiers&128 != 0 {
		if !e.Down || e.Repeat {
			return nil
		}
		on := 0
		if e.Modifiers&64 != 0 {
			on = 1
		}
		if status := C.yd_modifier_lock(C.kIOHIDNumLockState, C.int(on)); status != 0 {
			return fmt.Errorf("Num Lock 狀態同步失敗：%d", status)
		}
		return nil
	}
	numeric := 0
	name := rawkey.Name("darwin", code)
	if strings.HasPrefix(name, "kp") || (e.Platform == "darwin" && e.NativeFlags&(1<<21) != 0) {
		numeric = 1
	}
	nativeFlags := uint64(0)
	if e.Platform == "darwin" {
		nativeFlags = e.NativeFlags
		if code == 57 {
			// Reset 可能沿用按下時的旗標；以 Down 為準，確保放開不殘留。
			const physicalCaps = uint64(C.NX_ALPHASHIFT_STATELESS_MASK | C.NX_DEVICE_ALPHASHIFT_STATELESS_MASK)
			nativeFlags &^= physicalCaps
			if e.Down {
				nativeFlags |= physicalCaps
			}
		}
	}
	C.yd_raw_key(C.int(code), C.int(down), C.ulonglong(rawkey.TargetModifiers(e, "darwin")), C.int(repeat), C.int(numeric), C.ulonglong(nativeFlags))
	return nil
}
