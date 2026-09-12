package autostart

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const unitName = "yourdesk-autostart.service"

func paths() (string, string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", "", err
	}
	if !filepath.IsAbs(dir) {
		return "", "", os.ErrInvalid
	}
	return filepath.Join(dir, "autostart", "yourdesk.desktop"), filepath.Join(dir, "systemd", "user", unitName), nil
}
func supported(desktop bool) bool {
	if os.Geteuid() == 0 {
		return false
	}
	if _, _, err := paths(); err != nil {
		return false
	}
	if desktop {
		return true
	}
	_, err := exec.LookPath("systemctl")
	return err == nil
}
func enabled(desktop bool) (bool, error) {
	entry, unit, err := paths()
	if err != nil {
		return false, err
	}
	data, e := os.ReadFile(entry)
	if e == nil && !strings.Contains(string(data), "Hidden=true") {
		return true, nil
	}
	if e != nil && !os.IsNotExist(e) {
		return false, e
	}
	link := filepath.Join(filepath.Dir(unit), "default.target.wants", unitName)
	target, e := filepath.EvalSymlinks(link)
	if os.IsNotExist(e) {
		return false, nil
	}
	return e == nil && target == unit, e
}
func configure(ctx context.Context, on bool, c Config) error {
	entry, unit, err := paths()
	if err != nil {
		return err
	}
	link := filepath.Join(filepath.Dir(unit), "default.target.wants", unitName)
	if !on {
		// 只撤銷下次登入的啟動；不停止正在接受連線的 Host。
		for _, p := range []string{entry, link, unit} {
			if err = removeFile(p); err != nil {
				return err
			}
		}
		return nil
	}
	args := append([]string{c.Executable}, c.Args...)
	if c.Desktop {
		for i, arg := range args {
			arg = strings.ReplaceAll(arg, "%", "%%")
			arg = strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "`", "\\`", "$", "\\$").Replace(arg)
			args[i] = "\"" + arg + "\""
		}
		// Desktop Entry 的字串轉義先於 Exec 引號解析。
		command := strings.ReplaceAll(strings.Join(args, " "), "\\", "\\\\")
		if err = writeFile(entry, []byte("[Desktop Entry]\nType=Application\nName=YourDesk\nExec="+command+"\nTerminal=false\n")); err != nil {
			return err
		}
		return removeFile(link)
	}
	// 必須有可用的使用者 systemd；不設定 linger，不升級為 root 服務。
	if err = exec.CommandContext(ctx, "systemctl", "--user", "show-environment").Run(); err != nil {
		return errors.New(Unsupported)
	}
	for i, arg := range args {
		arg = strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "%", "%%", "$", "$$", "\t", "\\t").Replace(arg)
		args[i] = "\"" + arg + "\""
	}
	data := "[Unit]\nDescription=YourDesk login startup\n\n[Service]\nType=simple\nUnsetEnvironment=DISPLAY WAYLAND_DISPLAY\nExecStart=" + strings.Join(args, " ") + "\nRestart=on-failure\nRestartSec=5\n\n[Install]\nWantedBy=default.target\n"
	if err = writeFile(unit, []byte(data)); err != nil {
		return err
	}
	if err = exec.CommandContext(ctx, "systemctl", "--user", "daemon-reload").Run(); err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(link), 0700); err != nil {
		return err
	}
	if err = removeFile(link); err != nil {
		return err
	}
	if err = os.Symlink(unit, link); err != nil {
		return err
	}
	return removeFile(entry)
}
