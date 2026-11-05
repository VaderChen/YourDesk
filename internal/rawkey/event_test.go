package rawkey

import "testing"

func TestTranslateAndModifiers(t *testing.T) {
	// Mac -> Mac: Ctrl+F1 與 Ctrl+Left/Right 保留 Control 與對應鍵碼
	for _, key := range []string{"f1", "left", "right"} {
		code, ok := Code("darwin", key)
		if !ok {
			t.Fatalf("missing darwin key %s", key)
		}
		ev := Event{Platform: "darwin", Code: code, Modifiers: 2, Down: true}
		gotCode, err := TranslateEvent(ev, "darwin")
		if err != nil || gotCode != code {
			t.Fatalf("darwin->darwin %s code = %d, err = %v", key, gotCode, err)
		}
		if gotMods := TargetModifiers(ev, "darwin"); gotMods != 2 {
			t.Fatalf("darwin->darwin %s mods = %d, want 2", key, gotMods)
		}
	}

	// Mac -> Windows: Ctrl+F1 / Cmd+F1 / Ctrl+Left / Cmd+Left 都在 Windows 端成為 Ctrl (mods=2)
	for _, mod := range []uint64{2, 8} {
		for _, key := range []string{"f1", "left", "right"} {
			srcCode, _ := Code("darwin", key)
			wantCode, _ := Code("windows", key)
			ev := Event{Platform: "darwin", Code: srcCode, Modifiers: mod, Down: true}
			gotCode, err := TranslateEvent(ev, "windows")
			if err != nil || gotCode != wantCode {
				t.Fatalf("darwin->windows %s code = %d, want %d, err = %v", key, gotCode, wantCode, err)
			}
			if gotMods := TargetModifiers(ev, "windows"); gotMods != 2 {
				t.Fatalf("darwin->windows %s (mod %d) mods = %d, want 2", key, mod, gotMods)
			}
		}
	}

	// Windows -> Mac: Ctrl+F1 與 Ctrl+Left/Right 保留 Control (mods=2)，不誤轉為 Command
	for _, key := range []string{"f1", "f12", "left", "right", "up", "down", "home", "end", "pageup", "pagedown", "delete"} {
		srcCode, _ := Code("windows", key)
		wantCode, _ := Code("darwin", key)
		ev := Event{Platform: "windows", Code: srcCode, Modifiers: 2, Down: true}
		gotCode, err := TranslateEvent(ev, "darwin")
		if err != nil || gotCode != wantCode {
			t.Fatalf("windows->darwin %s code = %d, want %d, err = %v", key, gotCode, wantCode, err)
		}
		if gotMods := TargetModifiers(ev, "darwin"); gotMods != 2 {
			t.Fatalf("windows->darwin %s mods = %d, want 2 (Control)", key, gotMods)
		}
	}

	// Windows -> Mac: 一般文字熱鍵如 Ctrl+C 仍轉為 Command+C (mods=8)，而 Win+C 轉為 Control+C (mods=2)
	cWin, _ := Code("windows", "c")
	cMac, _ := Code("darwin", "c")
	ctrlCEv := Event{Platform: "windows", Code: cWin, Modifiers: 2, Down: true}
	if gotCode, err := TranslateEvent(ctrlCEv, "darwin"); err != nil || gotCode != cMac {
		t.Fatalf("windows->darwin c code = %d, err = %v", gotCode, err)
	}
	if gotMods := TargetModifiers(ctrlCEv, "darwin"); gotMods != 8 {
		t.Fatalf("windows->darwin Ctrl+C mods = %d, want 8 (Command)", gotMods)
	}
	winCEv := Event{Platform: "windows", Code: cWin, Modifiers: 8, Down: true}
	if gotMods := TargetModifiers(winCEv, "darwin"); gotMods != 2 {
		t.Fatalf("windows->darwin Win+C mods = %d, want 2 (Control)", gotMods)
	}

	// Windows -> Mac: 修飾鍵本體雙向對調 (Ctrl <-> Command, Win <-> Control)
	winLCtrl, _ := Code("windows", "leftcontrol")
	macLCmd, _ := Code("darwin", "leftsuper")
	if got, err := TranslateEvent(Event{Platform: "windows", Code: winLCtrl, Modifiers: 2, Down: true}, "darwin"); err != nil || got != macLCmd {
		t.Fatalf("windows->darwin leftcontrol = %d, want %d, err = %v", got, macLCmd, err)
	}
	winLSuper, _ := Code("windows", "leftsuper")
	macLCtrl, _ := Code("darwin", "leftcontrol")
	if got, err := TranslateEvent(Event{Platform: "windows", Code: winLSuper, Modifiers: 8, Down: true}, "darwin"); err != nil || got != macLCtrl {
		t.Fatalf("windows->darwin leftsuper = %d, want %d, err = %v", got, macLCtrl, err)
	}
}

func TestClipboardAndInputSourceChords(t *testing.T) {
	for _, tc := range []struct {
		src, dst, key string
		mods          uint64
		literal       bool
		want          string
	}{
		{"windows", "windows", "v", 8, false, ""}, {"windows", "windows", "v", 2, false, "v"},
		{"darwin", "windows", "v", 8, false, "v"}, {"windows", "darwin", "v", 2, false, "v"},
		{"windows", "darwin", "c", 2, true, ""}, {"darwin", "darwin", "c", 8, false, "c"},
		{"windows", "windows", "v", 6, false, ""}, {"darwin", "windows", "x", 8, false, "x"},
	} {
		code, _ := Code(tc.src, tc.key)
		e := Event{Platform: tc.src, Code: code, Modifiers: tc.mods, Down: true, DisableMapping: tc.literal}
		if got := ClipboardShortcut(e, tc.dst); got != tc.want {
			t.Errorf("%+v: got %q", tc, got)
		}
	}
	space, _ := Code("windows", "space")
	if got := TargetModifiers(Event{Platform: "windows", Code: space, Modifiers: 2}, "darwin"); got != 2 {
		t.Errorf("Ctrl+Space: mods=%v", got)
	}
}
