package filetransfer

import (
	"errors"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ntOpen only accepts a single name relative to an already-pinned directory.
// OPEN_REPARSE_POINT prevents a final symlink from being followed during a race.
func ntOpen(parent *os.File, name string, access uint32) (*os.File, error) {
	if name == "" || name == "." || name == ".." {
		return nil, errors.New("檔名無效")
	}
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return nil, errors.New("檔名無效")
	}
	oa := windows.OBJECT_ATTRIBUTES{RootDirectory: windows.Handle(parent.Fd()), ObjectName: objectName, Attributes: windows.OBJ_CASE_INSENSITIVE}
	oa.Length = uint32(unsafe.Sizeof(oa))
	var iosb windows.IO_STATUS_BLOCK
	var handle windows.Handle
	err = windows.NtCreateFile(&handle, access|windows.SYNCHRONIZE, &oa, &iosb, nil, 0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, windows.FILE_OPEN,
		windows.FILE_NON_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	if err != nil {
		return nil, errors.New("無法安全開啟檔案")
	}
	f := os.NewFile(uintptr(handle), name)
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		f.Close()
		return nil, errors.New("僅支援一般檔案")
	}
	return f, nil
}

func openRegular(root *os.Root, name string) (*os.File, error) {
	parent, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer parent.Close()
	return ntOpen(parent, name, windows.FILE_GENERIC_READ)
}

func openDirectory(root *os.Root, name string) (*os.File, error) { return root.Open(name) }
