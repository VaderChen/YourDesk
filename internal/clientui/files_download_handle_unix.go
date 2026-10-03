//go:build unix

package clientui

import (
	"os"

	"golang.org/x/sys/unix"
)

func duplicateDownloadFile(file *os.File) (*os.File, error) {
	raw, err := file.SyscallConn()
	if err != nil {
		return nil, err
	}
	var descriptor int
	var duplicateErr error
	if err := raw.Control(func(fd uintptr) {
		descriptor, duplicateErr = unix.FcntlInt(fd, unix.F_DUPFD_CLOEXEC, 0)
	}); err != nil {
		return nil, err
	}
	if duplicateErr != nil {
		return nil, duplicateErr
	}
	return os.NewFile(uintptr(descriptor), file.Name()), nil
}
