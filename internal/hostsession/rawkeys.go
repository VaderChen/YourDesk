package hostsession

import (
	"fmt"
	"runtime"
	"yourdesk/internal/input"
	"yourdesk/internal/rawkey"
)

// 進入 遠端顯示 時修飾鍵可能已按住，依事件快照補齊遠端狀態。
// 保留來源修飾鍵狀態；跨平台修飾鍵映射由注入層處理。
func reconcileRawModifiers(event rawkey.Event, held map[string]rawkey.Event) {
	name := rawkey.Name(event.Platform, event.Code)
	for _, modifier := range []struct {
		bit         uint64
		left, right string
	}{{1, "leftshift", "rightshift"}, {2, "leftcontrol", "rightcontrol"}, {4, "leftalt", "rightalt"}, {8, "leftsuper", "rightsuper"}} {
		if name == modifier.left || name == modifier.right {
			continue
		}
		present := false
		for id, key := range held {
			keyName := rawkey.Name(key.Platform, key.Code)
			if keyName != modifier.left && keyName != modifier.right {
				continue
			}
			present = true
			if event.Modifiers&modifier.bit == 0 {
				key.Down = false
				key.Repeat = false
				key.Modifiers = event.Modifiers
				_ = injectRawKey(key, held)
				delete(held, id)
			}
		}
		if !present && event.Modifiers&modifier.bit != 0 {
			if code, ok := rawkey.Code(event.Platform, modifier.left); ok {
				key := rawkey.Event{DisableMapping: event.DisableMapping, Platform: event.Platform, Code: code, Modifiers: event.Modifiers, Down: true}
				if injectRawKey(key, held) == nil {
					held[fmt.Sprintf("%s/%d", key.Platform, key.Code)] = key
				}
			}
		}
	}
}

// 多個來源鍵可能對應同一個遠端修飾鍵，最後一個來源鍵放開才放開遠端鍵。
func injectRawKey(event rawkey.Event, held map[string]rawkey.Event) error {
	if event.Platform != runtime.GOOS {
		target, err := rawkey.TranslateEvent(event, runtime.GOOS)
		if err != nil {
			return err
		}
		name := rawkey.Name(runtime.GOOS, target)
		if name == "leftcontrol" || name == "rightcontrol" || name == "leftsuper" || name == "rightsuper" {
			for _, other := range held {
				if !other.Down || (other.Platform == event.Platform && other.Code == event.Code) {
					continue
				}
				mapped, err := rawkey.TranslateEvent(other, runtime.GOOS)
				if err == nil && mapped == target {
					return nil
				}
			}
		}
	}
	return (input.Native{}).RawKey(event)
}
