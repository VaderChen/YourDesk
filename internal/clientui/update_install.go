package clientui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// 更新程序獨立於 APP 的生命週期；收到就緒訊號後，APP 才能退出。
func startUpdateHelper(ctx context.Context, cmd *exec.Cmd, dir string) error {
	logFile, err := os.OpenFile(filepath.Join(dir, "update.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer logFile.Close()
	cmd.Stdout, cmd.Stderr = logFile, logFile
	detachUpdateHelper(cmd)
	if err = cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timeout := time.NewTimer(15 * time.Second)
	defer timeout.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if _, err := os.Stat(filepath.Join(dir, "ready")); err == nil {
				return nil
			}
		case err := <-done:
			return fmt.Errorf("更新程序未就緒（%v），記錄：%s", err, filepath.Join(dir, "update.log"))
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			return ctx.Err()
		case <-timeout.C:
			_ = cmd.Process.Kill()
			return fmt.Errorf("更新程序啟動逾時，記錄：%s", filepath.Join(dir, "update.log"))
		}
	}
}
