//go:build !darwin && !windows && !linux

package autostart

import (
	"context"
	"errors"
)

func supported(bool) bool                           { return false }
func enabled(bool) (bool, error)                    { return false, nil }
func configure(context.Context, bool, Config) error { return errors.New(Unsupported) }
