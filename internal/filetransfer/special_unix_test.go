//go:build darwin || linux

package filetransfer

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestRejectSpecialFileWithoutBlocking(t *testing.T) {
	s, home := testSession(t)
	if err := syscall.Mkfifo(filepath.Join(home, "pipe"), 0600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(filepath.Join(home, "pipe"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := call(s, "read", map[string]any{"path": "pipe", "size": 0, "modified": entry("pipe", info).Modified}); err == nil {
		t.Fatal("FIFO accepted")
	}
	if _, err := call(s, "stat", map[string]string{"path": "pipe"}); err == nil {
		t.Fatal("FIFO stat accepted")
	}
	out := mustCall(t, s, "list", map[string]string{"path": "."}).(listResult)
	if len(out.Entries) != 0 {
		t.Fatal("FIFO listed")
	}
}
