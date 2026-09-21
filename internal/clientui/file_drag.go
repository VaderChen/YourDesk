package clientui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
	"unsafe"
)

// installFileDrag reserves a native drag source over the bottom 72 logical pixels
// of the window. Installation, updates and cleanup must run on its UI thread.
// Only completed local downloads may be supplied; their files must outlive any
// asynchronous Finder/Explorer copy, including after the source window closes.
func installFileDrag(window unsafe.Pointer) (func([]string) error, func(), error) {
	set, remove, err := nativeInstallFileDrag(window)
	if err != nil {
		return nil, nil, err
	}
	closed := false
	return func(paths []string) error {
			if closed {
				return errors.New("檔案拖曳視窗已關閉")
			}
			files, err := validatedFileDragPaths(paths)
			if err != nil {
				return err
			}
			return set(files)
		}, func() {
			if !closed {
				closed = true
				remove()
			}
		}, nil
}

func validatedFileDragPaths(paths []string) ([]string, error) {
	if len(paths) > 64 {
		return nil, errors.New("一次最多拖曳 64 個已下載檔案")
	}
	files := make([]string, 0, len(paths))
	seen := make(map[string]bool, len(paths))
	total := 0
	for _, name := range paths {
		total += len(name)
		if !filepath.IsAbs(name) || !utf8.ValidString(name) || strings.ContainsRune(name, 0) || total > 128*1024 {
			return nil, errors.New("拖曳來源路徑無效")
		}
		name = filepath.Clean(name)
		info, err := os.Lstat(name)
		if err != nil {
			return nil, fmt.Errorf("拖曳來源檔案不可用：%w", err)
		}
		if !info.Mode().IsRegular() {
			return nil, errors.New("僅能拖曳已完成下載的一般檔案")
		}
		if !seen[name] {
			seen[name] = true
			files = append(files, name)
		}
	}
	return files, nil
}
