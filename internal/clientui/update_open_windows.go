package clientui

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

func openUpdatePackage(ctx context.Context, path string) error {
	target := path
	if strings.EqualFold(filepath.Ext(path), ".zip") {
		dir, err := extractUpdatePackage(ctx, path)
		if err != nil {
			return err
		}
		target = dir
	} else if !strings.EqualFold(filepath.Ext(path), ".exe") {
		return fmt.Errorf("不支援的 Windows 更新套件")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	verb, _ := windows.UTF16PtrFromString("open")
	return windows.ShellExecute(0, verb, file, nil, nil, windows.SW_SHOWNORMAL)
}

// 每次解壓縮使用獨立目錄，失敗時移除半成品，避免覆蓋執行中的版本。
func extractUpdatePackage(ctx context.Context, path string) (string, error) {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer archive.Close()
	dir, err := os.MkdirTemp(filepath.Dir(path), strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))+"-")
	if err != nil {
		return "", err
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(dir)
		}
	}()
	remaining := int64(4 * maxUpdateSize)
	for _, entry := range archive.File {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		// IsLocal 同時拒絕 Windows 裝置名稱、磁碟機名稱及跳出目錄的路徑。
		name := filepath.FromSlash(entry.Name)
		if !filepath.IsLocal(name) || strings.Contains(name, ":") || entry.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("更新壓縮檔含有無效路徑")
		}
		target := filepath.Join(dir, name)
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0700); err != nil {
				return "", err
			}
			continue
		}
		if !entry.Mode().IsRegular() || entry.UncompressedSize64 > uint64(remaining) {
			return "", fmt.Errorf("更新壓縮檔內容無效或過大")
		}
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return "", err
		}
		if err := extractUpdateFile(ctx, entry, target, &remaining); err != nil {
			return "", err
		}
	}
	complete = true
	return dir, nil
}

func extractUpdateFile(ctx context.Context, entry *zip.File, target string, remaining *int64) error {
	source, err := entry.Open()
	if err != nil {
		return err
	}
	defer source.Close()
	output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer output.Close()
	// 分塊複製可讓結束 Client 時即時取消解壓縮。
	buffer := make([]byte, 128*1024)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, readErr := source.Read(buffer)
		if int64(n) > *remaining {
			return fmt.Errorf("更新壓縮檔內容過大")
		}
		if n > 0 {
			if _, err := output.Write(buffer[:n]); err != nil {
				return err
			}
			*remaining -= int64(n)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	return output.Close()
}
