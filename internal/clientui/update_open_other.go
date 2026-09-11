//go:build !darwin && !windows

package clientui

import (
	"context"
	"os/exec"
)

func openUpdatePackage(ctx context.Context, path string) error {
	return exec.CommandContext(ctx, "xdg-open", path).Run()
}
