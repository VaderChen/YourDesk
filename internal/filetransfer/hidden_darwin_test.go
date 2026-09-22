package filetransfer

import (
	"testing"

	"golang.org/x/sys/unix"
)

func setNativeHidden(t *testing.T, name string) {
	t.Helper()
	if err := unix.Chflags(name, unix.UF_HIDDEN); err != nil {
		t.Fatal(err)
	}
}
