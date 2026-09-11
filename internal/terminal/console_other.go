//go:build !darwin && !linux && !windows

package terminal

import "errors"

func supported() bool                 { return false }
func start(int, int) (console, error) { return nil, errors.New("此系統不支援終端機") }
