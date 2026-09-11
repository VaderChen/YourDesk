//go:build (!darwin && !windows) || (darwin && !cgo)

package main

import "yourdesk/internal/rawkey"

func rawKeyboardAvailable() bool         { return false }
func captureRawKeys(bool) []rawkey.Event { return nil }
