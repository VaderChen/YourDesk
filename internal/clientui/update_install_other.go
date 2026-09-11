//go:build !darwin && !windows

package clientui

import (
	"context"
	"fmt"
	"os/exec"
)

func detachUpdateHelper(cmd *exec.Cmd) {}
func prepareAutomaticUpdate(ctx context.Context, path string) error {
	return fmt.Errorf("此系統尚不支援自動安裝")
}
