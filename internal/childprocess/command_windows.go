//go:build windows

package childprocess

import (
	"os/exec"
	"syscall"
)

func configure(cmd *exec.Cmd) {
	// CREATE_NO_WINDOW 不為主控台程式建立視窗；GUI 程式會忽略此旗標。
	// 不設定 HideWindow，避免隱藏 Client／遠端顯示 的 GUI 主視窗。
	const createNoWindow = 0x08000000
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow}
}
