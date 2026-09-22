//go:build !windows

package prelogin

import (
	"context"
	"errors"
)

func runUpdateWorker(context.Context, string) error {
	return errors.New("此平台不使用 Windows 服務更新")
}
