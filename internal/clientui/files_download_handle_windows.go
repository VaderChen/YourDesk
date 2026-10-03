package clientui

import (
	"os"

	"golang.org/x/sys/windows"
)

func duplicateDownloadFile(file *os.File) (*os.File, error) {
	raw, err := file.SyscallConn()
	if err != nil {
		return nil, err
	}
	var handle windows.Handle
	var duplicateErr error
	if err := raw.Control(func(fd uintptr) {
		process := windows.CurrentProcess()
		duplicateErr = windows.DuplicateHandle(process, windows.Handle(fd), process, &handle, 0, false, windows.DUPLICATE_SAME_ACCESS)
	}); err != nil {
		return nil, err
	}
	if duplicateErr != nil {
		return nil, duplicateErr
	}
	return os.NewFile(uintptr(handle), file.Name()), nil
}
