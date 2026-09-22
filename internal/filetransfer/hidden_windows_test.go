package filetransfer

import (
	"testing"

	"golang.org/x/sys/windows"
)

func setNativeHidden(t *testing.T, name string) {
	t.Helper()
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		t.Fatal(err)
	}
	attributes, err := windows.GetFileAttributes(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetFileAttributes(p, attributes|windows.FILE_ATTRIBUTE_HIDDEN); err != nil {
		t.Fatal(err)
	}
}
