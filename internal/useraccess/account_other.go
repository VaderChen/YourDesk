//go:build !windows && !darwin && !linux

package useraccess

func Allowed() bool { return false }
