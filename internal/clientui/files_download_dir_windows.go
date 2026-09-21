//go:build windows

package clientui

import "golang.org/x/sys/windows"

func fileDownloadsDirectory() (string, error) {
	return windows.KnownFolderPath(windows.FOLDERID_Downloads, 0)
}
