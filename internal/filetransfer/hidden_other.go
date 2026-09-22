//go:build !darwin && !windows

package filetransfer

import "os"

func hiddenAttributes(info os.FileInfo) bool { return false }
