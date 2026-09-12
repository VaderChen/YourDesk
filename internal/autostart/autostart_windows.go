package autostart

import (
	"context"
	"errors"
	"golang.org/x/sys/windows/registry"
	"strings"
	"syscall"
	"unicode/utf16"
)

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const approvedKey = `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`

func supported(desktop bool) bool { return true }
func enabled(desktop bool) (bool, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer key.Close()
	value, _, err := key.GetStringValue("YourDesk")
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	approved, e := registry.OpenKey(registry.CURRENT_USER, approvedKey, registry.QUERY_VALUE)
	if e == nil {
		defer approved.Close()
		data, _, e := approved.GetBinaryValue("YourDesk")
		if e == nil && len(data) > 0 && data[0] == 3 {
			return false, nil
		}
	}
	return value != "", nil
}
func configure(ctx context.Context, on bool, c Config) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	if !on {
		err = key.DeleteValue("YourDesk")
		if errors.Is(err, registry.ErrNotExist) {
			err = nil
		}
		return err
	}
	args := append([]string{c.Executable}, c.Args...)
	for i, arg := range args {
		args[i] = syscall.EscapeArg(arg)
	}
	command := strings.Join(args, " ")
	if len(utf16.Encode([]rune(command))) > 260 {
		return errors.New(Failed)
	}
	// 使用者在本程式明確開啟時，清除工作管理員先前的停用記錄。
	approved, e := registry.OpenKey(registry.CURRENT_USER, approvedKey, registry.SET_VALUE)
	if e == nil {
		defer approved.Close()
		e = approved.DeleteValue("YourDesk")
		if e != nil && !errors.Is(e, registry.ErrNotExist) {
			return e
		}
	} else if !errors.Is(e, registry.ErrNotExist) {
		return e
	}
	return key.SetStringValue("YourDesk", command)
}
