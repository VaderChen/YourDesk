package hostguard

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"yourdesk/internal/deviceid"
)

// 舊版没有鎖資料時，從 Windows 查詢既有 Host。命令列僅在記憶體比對，不回傳介面或 LOG。
func FindExisting(ctx context.Context, room, signal string, excluded map[int]bool) ([]Owner, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	script := `$ErrorActionPreference='Stop'; [Console]::OutputEncoding=[System.Text.UTF8Encoding]::new($false); $items=@(Get-CimInstance Win32_Process -Filter "Name='yourdesk-client.exe'" | ForEach-Object { $owner=Invoke-CimMethod -InputObject $_ -MethodName GetOwnerSid; [pscustomobject]@{PID=$_.ProcessId;CommandLine=$_.CommandLine;SID=$owner.Sid} }); ConvertTo-Json -InputObject $items -Compress`
	executable := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	cmd := exec.CommandContext(ctx, executable, "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	raw, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var rows []struct {
		PID              int
		CommandLine, SID string
	}
	if err = json.Unmarshal(raw, &rows); err != nil {
		return nil, err
	}
	current, err := user.Current()
	if err != nil {
		return nil, err
	}
	localRoom := ""
	if identity, e := deviceid.Current(); e == nil {
		localRoom = identity.UID
	}
	var matches []Owner
	for _, row := range rows {
		if excluded[row.PID] || row.PID == os.Getpid() || row.SID != current.Uid || row.CommandLine == "" {
			continue
		}
		args, e := windows.DecomposeCommandLine(row.CommandLine)
		if e != nil || len(args) == 0 {
			continue
		}
		candidateRoom, candidateSignal, isHost := hostArguments(args[1:], localRoom)
		if !isHost || candidateRoom != room || candidateSignal != signal {
			continue
		}
		owner, e := processOwner(row.PID)
		if e == nil && strings.EqualFold(filepath.Base(owner.Path), "yourdesk-client.exe") {
			matches = append(matches, owner)
		}
	}
	return matches, nil
}
