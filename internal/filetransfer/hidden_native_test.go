//go:build darwin || windows

package filetransfer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListingNativeHiddenAttributes(t *testing.T) {
	s, home := testSession(t)
	for _, directory := range []bool{false, true} {
		name := "hidden-file.txt"
		if directory {
			name = "hidden-folder"
		}
		p := filepath.Join(home, name)
		var err error
		if directory {
			err = os.Mkdir(p, 0700)
		} else {
			err = os.WriteFile(p, nil, 0600)
		}
		if err != nil {
			t.Fatal(err)
		}
		setNativeHidden(t, p)
	}
	if err := os.WriteFile(filepath.Join(home, "visible.txt"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	out := mustCall(t, s, "list", map[string]any{}).(listResult)
	if len(out.Entries) != 1 || out.Entries[0].Name != "visible.txt" || out.NextOffset != -1 {
		t.Fatalf("原生隱藏屬性未生效：%+v", out)
	}
}
