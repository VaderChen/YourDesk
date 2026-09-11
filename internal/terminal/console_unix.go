//go:build darwin || linux

package terminal

import (
	"github.com/creack/pty"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
)

type unixConsole struct {
	*os.File
	cmd    *exec.Cmd
	once   sync.Once
	exited atomic.Bool
}

func supported() bool { return true }
func start(cols, rows int) (console, error) {
	shell := os.Getenv("SHELL")
	if !filepath.IsAbs(shell) {
		shell = "/bin/sh"
	}
	cmd := exec.Command(shell, "-l")
	cmd.Dir, _ = os.UserHomeDir()
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")
	f, e := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if e != nil {
		return nil, e
	}
	return &unixConsole{File: f, cmd: cmd}, nil
}
func (c *unixConsole) Resize(cols, rows int) error {
	return pty.Setsize(c.File, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}
func (c *unixConsole) Wait() error { err := c.cmd.Wait(); c.exited.Store(true); return err }
func (c *unixConsole) Close() error {
	c.once.Do(func() {
		if !c.exited.Load() {
			_ = syscall.Kill(-c.cmd.Process.Pid, syscall.SIGKILL)
		}
		c.File.Close()
	})
	return nil
}
