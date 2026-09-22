//go:build !windows

package clientui

import (
	"os"
)

func fileDownloadsDirectory() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return home, nil
}
