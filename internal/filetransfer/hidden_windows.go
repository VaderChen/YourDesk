package filetransfer

import (
	"os"
	"syscall"
)

func hiddenAttributes(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Win32FileAttributeData)
	return ok && stat.FileAttributes&syscall.FILE_ATTRIBUTE_HIDDEN != 0
}
