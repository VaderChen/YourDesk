package hardwareprobe

import "golang.org/x/sys/unix"

func physicalMemoryBytes() any {
	var info unix.Sysinfo_t
	if unix.Sysinfo(&info) != nil || info.Totalram == 0 || info.Unit == 0 {
		return nil
	}
	return uint64(info.Totalram) * uint64(info.Unit)
}
