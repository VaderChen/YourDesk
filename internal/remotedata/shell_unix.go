//go:build !windows

package remotedata

import (
	"context"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func openRegular(root *os.Root, name string) (*os.File, error) {
	return checkRegular(root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0))
}
func runShell(ctx context.Context, in Shell) (any, error) {
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", in.Command)
	cmd.Dir = in.Directory
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 200 * time.Millisecond
	var out outputBuffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	defer syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	err := cmd.Wait()
	code := 0
	if err != nil {
		code = -1
		if cmd.ProcessState != nil {
			code = cmd.ProcessState.ExitCode()
		}
	}
	return out.result(code, ctx.Err() != nil, "/bin/sh"), nil
}

func openDirectory(root *os.Root, name string) (*os.File, error) {
	return root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
}
