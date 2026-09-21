//go:build !darwin && !linux && !windows

package filetransfer

import (
	"errors"
	"os"
)

func commitNoReplace(root *os.Root, temp, name string, expected os.FileInfo) error {
	info, err := root.Lstat(temp)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || !os.SameFile(info, expected) {
		return errors.New("暫存檔案已變更")
	}
	return root.Link(temp, name)
}
