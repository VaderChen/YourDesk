//go:build darwin

package deviceid

import (
	"errors"
	"os/exec"
	"regexp"
)

// Apple does not expose a stable unique CPU serial. Returning unavailable is
// deliberate: a CPU model/family is not unique and would cause UID collisions.
func cpuID() (string, error) { return "", errors.New("macOS 不提供唯一 CPU ID") }

func mainboardID() (string, error) {
	out, err := exec.Command("/usr/sbin/ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output()
	if err != nil {
		return "", err
	}
	for _, key := range []string{"IOPlatformSerialNumber", "IOPlatformUUID"} {
		re := regexp.MustCompile(`"` + key + `"\s*=\s*"([^"]+)"`)
		if match := re.FindSubmatch(out); len(match) == 2 {
			return string(match[1]), nil
		}
	}
	return "", errors.New("找不到 Apple platform identifier")
}
