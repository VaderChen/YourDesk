package filetransfer

import (
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func hiddenAttributes(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Flags&unix.UF_HIDDEN != 0
}
