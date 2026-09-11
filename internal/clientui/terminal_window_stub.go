//go:build !cgo || (!darwin && !windows && !linux)

package clientui

import "context"

func RunTerminalWindow(context.Context) error { return windowAvailable() }
