package hostguard

import (
	"errors"
	"golang.org/x/sys/windows"
)

func ownerFromHandle(h windows.Handle, pid int) (Owner, error) {
	var created, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &created, &exit, &kernel, &user); err != nil {
		return Owner{}, err
	}
	if exit.HighDateTime != 0 || exit.LowDateTime != 0 {
		return Owner{}, errors.New("程序已結束")
	}
	buf := make([]uint16, 32768)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return Owner{}, err
	}
	return Owner{pid, windows.UTF16ToString(buf[:size]), uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime)}, nil
}
func processOwner(pid int) (Owner, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return Owner{}, err
	}
	defer windows.CloseHandle(h)
	return ownerFromHandle(h, pid)
}
func stopVerified(owner Owner) error {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE, false, uint32(owner.PID))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	actual, err := ownerFromHandle(h, owner.PID)
	if err != nil {
		return err
	}
	if actual != owner {
		return errors.New("程序身分已變更，未停止任何程序")
	}
	return windows.TerminateProcess(h, 0)
}
