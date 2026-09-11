// 掃描碼對照來源：GLFW／Ebitengine（Apache-2.0）。
// Copyright 2009-2019 Camilla Löwy; 2023 The Ebitengine Authors.
package rawkey

import "fmt"

// Code 保留來源平台原始掃描碼；Windows 第 8 位表示 E0 擴充鍵。
// Modifiers: Shift=1、Control=2、Alt=4、Meta=8、CapsLock=16、Fn=32、NumLock=64、NumLock 狀態有效=128。
type Event struct {
	DisableMapping bool   `json:"disableMapping,omitempty"`
	Platform       string `json:"platform"`
	Code           int    `json:"code"`
	Modifiers      uint64 `json:"modifiers"`
	NativeFlags    uint64 `json:"nativeFlags"`
	Down           bool   `json:"down"`
	Repeat         bool   `json:"repeat,omitempty"`
	Reset          bool   `json:"reset,omitempty"`
}

func Name(platform string, code int) string { return scanNames[platform][code] }
func Translate(platform string, code int, target string) (int, error) {
	if code < 0 || code > 511 {
		return 0, fmt.Errorf("無效掃描碼")
	}
	if platform == target && (target == "windows" || code <= 127) {
		return code, nil
	}
	name := Name(platform, code)
	if name != "" {
		for other, value := range scanNames[target] {
			if value == name {
				return other, nil
			}
		}
	}
	return 0, fmt.Errorf("遠端平台無對應實體按鍵：%s/%d", platform, code)
}

var scanNames = map[string]map[int]string{
	"darwin": {
		0:   "a",
		1:   "s",
		2:   "d",
		3:   "f",
		4:   "h",
		5:   "g",
		6:   "z",
		7:   "x",
		8:   "c",
		9:   "v",
		10:  "world1",
		11:  "b",
		12:  "q",
		13:  "w",
		14:  "e",
		15:  "r",
		16:  "y",
		17:  "t",
		18:  "1",
		19:  "2",
		20:  "3",
		21:  "4",
		22:  "6",
		23:  "5",
		24:  "equal",
		25:  "9",
		26:  "7",
		27:  "minus",
		28:  "8",
		29:  "0",
		30:  "rightbracket",
		31:  "o",
		32:  "u",
		33:  "leftbracket",
		34:  "i",
		35:  "p",
		36:  "enter",
		37:  "l",
		38:  "j",
		39:  "apostrophe",
		40:  "k",
		41:  "semicolon",
		42:  "backslash",
		43:  "comma",
		44:  "slash",
		45:  "n",
		46:  "m",
		47:  "period",
		48:  "tab",
		49:  "space",
		50:  "graveaccent",
		51:  "backspace",
		53:  "escape",
		54:  "rightsuper",
		55:  "leftsuper",
		56:  "leftshift",
		57:  "capslock",
		58:  "leftalt",
		59:  "leftcontrol",
		60:  "rightshift",
		61:  "rightalt",
		62:  "rightcontrol",
		64:  "f17",
		65:  "kpdecimal",
		67:  "kpmultiply",
		69:  "kpadd",
		71:  "numlock",
		75:  "kpdivide",
		76:  "kpenter",
		78:  "kpsubtract",
		79:  "f18",
		80:  "f19",
		81:  "kpequal",
		82:  "kp0",
		83:  "kp1",
		84:  "kp2",
		85:  "kp3",
		86:  "kp4",
		87:  "kp5",
		88:  "kp6",
		89:  "kp7",
		90:  "f20",
		91:  "kp8",
		92:  "kp9",
		96:  "f5",
		97:  "f6",
		98:  "f7",
		99:  "f3",
		100: "f8",
		101: "f9",
		103: "f11",
		105: "printscreen",
		106: "f16",
		107: "f14",
		109: "f10",
		110: "menu",
		111: "f12",
		113: "f15",
		114: "insert",
		115: "home",
		116: "pageup",
		117: "delete",
		118: "f4",
		119: "end",
		120: "f2",
		121: "pagedown",
		122: "f1",
		123: "left",
		124: "right",
		125: "down",
		126: "up",
	},
	"windows": {
		1:   "escape",
		2:   "1",
		3:   "2",
		4:   "3",
		5:   "4",
		6:   "5",
		7:   "6",
		8:   "7",
		9:   "8",
		10:  "9",
		11:  "0",
		12:  "minus",
		13:  "equal",
		14:  "backspace",
		15:  "tab",
		16:  "q",
		17:  "w",
		18:  "e",
		19:  "r",
		20:  "t",
		21:  "y",
		22:  "u",
		23:  "i",
		24:  "o",
		25:  "p",
		26:  "leftbracket",
		27:  "rightbracket",
		28:  "enter",
		29:  "leftcontrol",
		30:  "a",
		31:  "s",
		32:  "d",
		33:  "f",
		34:  "g",
		35:  "h",
		36:  "j",
		37:  "k",
		38:  "l",
		39:  "semicolon",
		40:  "apostrophe",
		41:  "graveaccent",
		42:  "leftshift",
		43:  "backslash",
		44:  "z",
		45:  "x",
		46:  "c",
		47:  "v",
		48:  "b",
		49:  "n",
		50:  "m",
		51:  "comma",
		52:  "period",
		53:  "slash",
		54:  "rightshift",
		55:  "kpmultiply",
		56:  "leftalt",
		57:  "space",
		58:  "capslock",
		59:  "f1",
		60:  "f2",
		61:  "f3",
		62:  "f4",
		63:  "f5",
		64:  "f6",
		65:  "f7",
		66:  "f8",
		67:  "f9",
		68:  "f10",
		69:  "pause",
		70:  "scrolllock",
		71:  "kp7",
		72:  "kp8",
		73:  "kp9",
		74:  "kpsubtract",
		75:  "kp4",
		76:  "kp5",
		77:  "kp6",
		78:  "kpadd",
		79:  "kp1",
		80:  "kp2",
		81:  "kp3",
		82:  "kp0",
		83:  "kpdecimal",
		86:  "world2",
		87:  "f11",
		88:  "f12",
		89:  "kpequal",
		100: "f13",
		101: "f14",
		102: "f15",
		103: "f16",
		104: "f17",
		105: "f18",
		106: "f19",
		107: "f20",
		108: "f21",
		109: "f22",
		110: "f23",
		118: "f24",
		284: "kpenter",
		285: "rightcontrol",
		309: "kpdivide",
		311: "printscreen",
		312: "rightalt",
		325: "numlock",
		327: "home",
		328: "up",
		329: "pageup",
		331: "left",
		333: "right",
		335: "end",
		336: "down",
		337: "pagedown",
		338: "insert",
		339: "delete",
		347: "leftsuper",
		348: "rightsuper",
		349: "menu",
	},
}

func Code(platform, name string) (int, bool) {
	for code, value := range scanNames[platform] {
		if value == name {
			return code, true
		}
	}
	return 0, false
}

// TranslateEvent 將使用者選擇的跨平台修飾鍵映射套用於原始來源事件。
func TranslateEvent(e Event, target string) (int, error) {
	name := Name(e.Platform, e.Code)
	if !e.DisableMapping {
		if e.Platform == "darwin" && target == "windows" {
			if name == "leftsuper" {
				name = "leftcontrol"
			}
			if name == "rightsuper" {
				name = "rightcontrol"
			}
		}
		if e.Platform == "windows" && target == "darwin" {
			if name == "leftcontrol" {
				name = "leftsuper"
			}
			if name == "rightcontrol" {
				name = "rightsuper"
			}
		}
	}
	if name != Name(e.Platform, e.Code) {
		if code, ok := Code(target, name); ok {
			return code, nil
		}
	}
	return Translate(e.Platform, e.Code, target)
}
func TargetModifiers(e Event, target string) uint64 {
	mods := e.Modifiers
	if !e.DisableMapping {
		if e.Platform == "darwin" && target == "windows" && mods&8 != 0 {
			mods = (mods &^ 8) | 2
		}
		if e.Platform == "windows" && target == "darwin" && mods&2 != 0 {
			mods = (mods &^ 2) | 8
		}
	}
	return mods
}
func LegacyKey(platform, target, key string, disabled bool) string {
	if !disabled {
		if platform == "darwin" && target == "windows" && key == "meta" {
			return "control"
		}
		if platform == "windows" && target == "darwin" && key == "control" {
			return "meta"
		}
	}
	return key
}
