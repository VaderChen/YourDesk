package filetransfer

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
)

func commitNoReplace(root *os.Root, temp, name string, expected os.FileInfo) error {
	f, err := openDirectory(root, ".")
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := root.Lstat(temp)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || !os.SameFile(info, expected) {
		return errors.New("暫存檔案已變更")
	}
	if err := unix.Renameat2(int(f.Fd()), temp, int(f.Fd()), name, unix.RENAME_NOREPLACE); err != nil {
		return errors.New("無法提交檔案：目的已存在或檔案系統不支援安全更名")
	}
	return nil
}
