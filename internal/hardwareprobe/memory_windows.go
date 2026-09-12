package hardwareprobe

import (
	"golang.org/x/sys/windows"
	"unsafe"
)

// MEMORYSTATUSEX 回報可供作業系統使用的實體記憶體總容量，不是剩餘容量。
func physicalMemoryBytes() any {
	var status struct {
		Length, Load                                       uint32
		TotalPhys, AvailPhys, TotalPageFile, AvailPageFile uint64
		TotalVirtual, AvailVirtual, AvailExtendedVirtual   uint64
	}
	status.Length = uint32(unsafe.Sizeof(status))
	proc := windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")
	if err := proc.Find(); err != nil {
		return nil
	}
	ok, _, _ := proc.Call(uintptr(unsafe.Pointer(&status)))
	if ok == 0 || status.TotalPhys == 0 {
		return nil
	}
	return status.TotalPhys
}
