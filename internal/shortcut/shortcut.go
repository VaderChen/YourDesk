// Package shortcut 分類需要選擇作用對象的快捷鍵，不攔截一般編輯操作。
package shortcut

import "yourdesk/internal/rawkey"

type Request struct {
	Platform    string
	Key         string
	Modifiers   uint64
	Label       string
	LocalAction int
	Secure      bool
}

func Match(platform, key string, mods uint64) (Request, bool) {
	mods &= 15
	r := Request{Platform: platform, Key: key, Modifiers: mods}
	switch {
	case key == "delete" && mods == 6:
		r.Label, r.Secure = "Ctrl+Alt+Del", true
	case platform == "darwin" && key == "q" && mods == 8:
		r.Label, r.LocalAction = "Cmd+Q", 13
	case platform == "darwin" && key == "w" && mods == 8:
		r.Label, r.LocalAction = "Cmd+W", 5
	case platform != "darwin" && key == "w" && mods == 2:
		r.Label, r.LocalAction = "Ctrl+W", 5
	case platform == "windows" && key == "f4" && mods == 4:
		r.Label, r.LocalAction = "Alt+F4", 5
	case platform == "windows" && key == "escape" && mods == 3:
		r.Label = "Ctrl+Shift+Esc"
	default:
		return Request{}, false
	}
	return r, true
}

// Chord 於確認後才建立完整的按下／放開序列，不重播等待期間的鍵盤事件。
// 安全注意序列不能使用一般鍵盤注入，必須明確拒絕。
func (r Request) Chord() []rawkey.Event {
	if r.Secure {
		return nil
	}
	key, ok := rawkey.Code(r.Platform, r.Key)
	if !ok {
		return nil
	}
	var events []rawkey.Event
	var mods uint64
	type modifier struct {
		name string
		bit  uint64
	}
	order := []modifier{{"leftshift", 1}, {"leftcontrol", 2}, {"leftalt", 4}, {"leftsuper", 8}}
	appendKey := func(code int, down bool) {
		events = append(events, rawkey.Event{Platform: r.Platform, Code: code, Modifiers: mods, Down: down})
	}
	for _, m := range order {
		if r.Modifiers&m.bit != 0 {
			mods |= m.bit
			code, _ := rawkey.Code(r.Platform, m.name)
			appendKey(code, true)
		}
	}
	appendKey(key, true)
	appendKey(key, false)
	for i := len(order) - 1; i >= 0; i-- {
		m := order[i]
		if mods&m.bit != 0 {
			mods &^= m.bit
			code, _ := rawkey.Code(r.Platform, m.name)
			appendKey(code, false)
		}
	}
	return events
}
