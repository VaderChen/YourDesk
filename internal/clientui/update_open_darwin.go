package clientui

import (
	"context"
	"os/exec"
)

func openUpdatePackage(ctx context.Context, path string) error {
	return exec.CommandContext(ctx, "/usr/bin/open", path).Run()
}
