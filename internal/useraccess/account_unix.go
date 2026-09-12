//go:build darwin || linux

package useraccess

import "os"

// Allowed 拒絕 root 與未知帳號。
func Allowed() bool { return os.Geteuid() > 0 }
