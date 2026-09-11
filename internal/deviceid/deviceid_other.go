//go:build !darwin && !windows

package deviceid

import (
	"errors"
	"os"
	"strings"
)

func cpuID() (string, error) { return readFirst("/sys/devices/virtual/dmi/id/product_uuid") }
func mainboardID() (string, error) {
	return readFirst("/sys/devices/virtual/dmi/id/board_serial", "/etc/machine-id")
}
func readFirst(paths ...string) (string, error) {
	for _, p := range paths {
		if b, err := os.ReadFile(p); err == nil && strings.TrimSpace(string(b)) != "" {
			return strings.TrimSpace(string(b)), nil
		}
	}
	return "", errors.New("hardware identifier unavailable")
}
