package filetransfer

import (
	"errors"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

type renameInformation struct {
	ReplaceIfExists uint32
	RootDirectory   windows.Handle
	FileNameLength  uint32
	FileName        [1]uint16
}

func commitNoReplace(root *os.Root, temp, name string, expected os.FileInfo) error {
	parent, err := root.Open(".")
	if err != nil {
		return err
	}
	defer parent.Close()
	f, err := ntOpen(parent, temp, windows.DELETE|windows.FILE_READ_ATTRIBUTES)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(info, expected) {
		return errors.New("暫存檔案已變更")
	}
	encoded, err := windows.UTF16FromString(name)
	if err != nil {
		return errors.New("檔名無效")
	}
	encoded = encoded[:len(encoded)-1]
	var header renameInformation
	buffer := make([]byte, int(unsafe.Offsetof(header.FileName))+len(encoded)*2)
	rename := (*renameInformation)(unsafe.Pointer(&buffer[0]))
	// ReplaceIfExists deliberately stays zero: never overwrite an existing file.
	rename.RootDirectory = windows.Handle(parent.Fd())
	rename.FileNameLength = uint32(len(encoded) * 2)
	copy(unsafe.Slice(&rename.FileName[0], len(encoded)), encoded)
	var iosb windows.IO_STATUS_BLOCK
	err = windows.NtSetInformationFile(windows.Handle(f.Fd()), &iosb, &buffer[0], uint32(len(buffer)), windows.FileRenameInformation)
	runtime.KeepAlive(parent)
	runtime.KeepAlive(f)
	if err != nil {
		return errors.New("無法提交檔案：目的已存在或檔案系統不支援安全更名")
	}
	return nil
}
