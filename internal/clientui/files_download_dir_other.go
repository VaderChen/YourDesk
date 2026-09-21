//go:build !windows

package clientui

import (
	"os"
	"path/filepath"
)

func fileDownloadsDirectory() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Downloads"), nil
}
