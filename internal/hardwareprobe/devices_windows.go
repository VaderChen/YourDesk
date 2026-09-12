package hardwareprobe

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// 在既有受逾時保護的 inventory helper 中查詢，不呼叫外部命令。
func platformDeviceNames() (map[string]string, []map[string]string) {
	var processor map[string]string
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\CentralProcessor\0`, registry.QUERY_VALUE)
	if err == nil {
		name, _, err := key.GetStringValue("ProcessorNameString")
		key.Close()
		if name = strings.TrimSpace(name); err == nil && name != "" {
			processor = map[string]string{"brand": name}
		}
	}
	var gpus []map[string]string
	proc := windows.NewLazySystemDLL("user32.dll").NewProc("EnumDisplayDevicesW")
	if proc.Find() != nil {
		return processor, gpus
	}
	seen := make(map[string]bool)
	for index := uint32(0); ; index++ {
		// DISPLAY_DEVICEW，包含 UTF-16 固定長度欄位；32/64 位元佈局相同。
		var device struct {
			Size        uint32
			Name        [32]uint16
			Description [128]uint16
			Flags       uint32
			ID          [128]uint16
			Key         [128]uint16
		}
		device.Size = uint32(unsafe.Sizeof(device))
		ok, _, _ := proc.Call(0, uintptr(index), uintptr(unsafe.Pointer(&device)), 0)
		if ok == 0 {
			break
		}
		// 不列入鏡像驅動；保留未接螢幕的顯示卡，不以桌面啟用旗標篩選。
		if device.Flags&0x8 != 0 {
			continue
		}
		name := strings.TrimSpace(windows.UTF16ToString(device.Description[:]))
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		gpus = append(gpus, map[string]string{"name": name})
	}
	return processor, gpus
}
