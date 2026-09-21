//go:build darwin || linux

package filetransfer

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestRemovePreflightRejectsFIFO(t *testing.T) {
	s, home := testSession(t)
	if err := os.Mkdir(filepath.Join(home, "special"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "special", "regular"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(home, "special", "pipe"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := call(s, "remove", removeArgs(t, s, "special", true)); err == nil {
		t.Fatal("remove accepted special file")
	}
	if _, err := os.Stat(filepath.Join(home, "special", "regular")); err != nil {
		t.Fatal("special-file preflight partially deleted files")
	}
}
