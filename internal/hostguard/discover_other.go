//go:build !windows

package hostguard

import "context"

func FindExisting(context.Context, string, string, map[int]bool) ([]Owner, error) { return nil, nil }
