//go:build windows

// Package useraccess 統一遠端資料與 Shell 的帳號安全檢查。
package useraccess

import (
	"golang.org/x/sys/windows"
	"os"
)

// Allowed 不以 Windows 的 Geteuid 判定 SYSTEM；查詢失敗一律拒絕。
func Allowed() bool {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil || user == nil || user.User.Sid == nil {
		return false
	}
	if user.User.Sid.IsWellKnown(windows.WinLocalSystemSid) || user.User.Sid.IsWellKnown(windows.WinLocalServiceSid) || user.User.Sid.IsWellKnown(windows.WinNetworkServiceSid) {
		return false
	}
	var session uint32
	return windows.ProcessIdToSessionId(uint32(os.Getpid()), &session) == nil && session != 0 && session != 0xffffffff
}
