//go:build windows && !cgo

package desktop

func newDesktopGPU() desktopGPU { return nil }
