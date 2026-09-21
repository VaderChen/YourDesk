package desktop

import (
	"errors"
	"reflect"
	"testing"
)

func TestCaptureDesktopSwitch(t *testing.T) {
	var events []string
	var next uintptr
	name := "Default"
	denied := false
	b := captureDesktopBinding{
		open: func() (uintptr, string, error) { next++; return next, name, nil },
		bind: func(uintptr) error {
			events = append(events, "bind")
			if denied {
				return errors.New("denied")
			}
			return nil
		},
		release: func(uintptr) { events = append(events, "close") },
	}
	reset := func() { events = append(events, "reset") }
	for _, step := range []struct {
		name   string
		denied bool
		want   []string
	}{
		{"Default", false, []string{"reset", "bind"}},
		{"default", false, []string{"close"}},
		{"Winlogon", false, []string{"reset", "bind", "close"}},
		{"Default", true, []string{"reset", "bind", "close"}},
		{"Default", false, []string{"reset", "bind", "close"}},
	} {
		name, denied = step.name, step.denied
		events = nil
		previous := b.current
		err := b.ensure(reset)
		if (err != nil) != denied {
			t.Fatalf("%s: %v", name, err)
		}
		if denied && (b.current != previous || b.name != "Winlogon") {
			t.Fatal("切換失敗覆蓋了有效桌面")
		}
		if !reflect.DeepEqual(events, step.want) {
			t.Fatalf("%s: %v want %v", name, events, step.want)
		}
	}
	b.open = func() (uintptr, string, error) { return 0, "", errors.New("no desktop") }
	events = nil
	if b.ensure(reset) == nil || len(events) != 0 {
		t.Fatal("桌面不可用時仍執行擷取資源切換")
	}
}
