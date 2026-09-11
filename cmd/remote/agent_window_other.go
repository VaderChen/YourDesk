//go:build (!darwin && !windows) || !cgo

package main

func nativeHideAgentWindow() {}
func nativeShowAgentWindow() {}
