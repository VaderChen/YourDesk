package shortcut

import (
	"testing"
	"yourdesk/internal/rawkey"
)

func TestSystemAndEditingShortcuts(t *testing.T) {
	for _, tc := range []struct {
		platform, key string
		mods          uint64
		match         bool
	}{
		{"darwin", "q", 8, true}, {"darwin", "w", 8, true}, {"windows", "w", 2, true},
		{"windows", "f4", 4, true}, {"windows", "escape", 3, true}, {"darwin", "delete", 6, true},
		{"windows", "delete", 6, true}, {"windows", "c", 2, false}, {"windows", "v", 2, false},
		{"darwin", "c", 8, false}, {"darwin", "v", 8, false}, {"darwin", "q", 0, false},
		{"darwin", "q", 9, false}, {"windows", "w", 18, true},
	} {
		if _, ok := Match(tc.platform, tc.key, tc.mods); ok != tc.match {
			t.Errorf("%+v: matched=%v", tc, ok)
		}
	}
}

func TestConfirmedChordReleasesEveryKey(t *testing.T) {
	for _, tc := range []struct {
		platform, key string
		mods          uint64
	}{
		{"darwin", "q", 8}, {"windows", "escape", 3}, {"windows", "f4", 4},
	} {
		r, _ := Match(tc.platform, tc.key, tc.mods)
		held := map[int]bool{}
		presses := 0
		for _, e := range r.Chord() {
			if e.Repeat || e.Reset {
				t.Fatal("不應重播狀態事件", e)
			}
			if e.Down {
				if held[e.Code] {
					t.Fatal("重複按下", e)
				}
				held[e.Code] = true
				if rawkey.Name(e.Platform, e.Code) == tc.key {
					presses++
					if e.Modifiers != tc.mods {
						t.Fatal("修飾鍵未先按下", e)
					}
				}
			} else {
				if !held[e.Code] {
					t.Fatal("沒有對應的按下", e)
				}
				delete(held, e.Code)
			}
		}
		if len(held) != 0 || presses != 1 {
			t.Fatal("快捷鍵未完整釋放", r, held)
		}
		last := r.Chord()
		if last[len(last)-1].Modifiers != 0 {
			t.Fatal("殘留修飾鍵")
		}
	}
	r, _ := Match("windows", "delete", 6)
	if len(r.Chord()) != 0 {
		t.Fatal("不能用一般輸入合成安全注意序列")
	}
}
