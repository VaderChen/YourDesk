package clientui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"yourdesk/internal/prelogin"
)

func detachUpdateHelper(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x00000200}
}
func prepareAutomaticUpdate(ctx context.Context, archive string) error {
	if prelogin.Status().Enabled {
		return fmt.Errorf("請先停用未登入開機，再更新 APP；更新後重新啟用服務。")
	}
	if !strings.HasSuffix(strings.ToLower(archive), "-setup.exe") {
		return fmt.Errorf("此套件不支援自動安裝，請手動開啟下載檔案")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	target := filepath.Dir(exe)
	if _, err = os.Stat(filepath.Join(target, "YourDesk.exe")); err != nil || os.Getenv("YOURDESK_TEST_UPDATE") == "1" {
		target = filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "YourDesk")
	}
	if !filepath.IsAbs(target) {
		return fmt.Errorf("無法確認安裝位置")
	}
	if err = os.MkdirAll(target, 0700); err != nil {
		return fmt.Errorf("無法寫入安裝位置：%w", err)
	}
	probe, err := os.CreateTemp(target, ".update-write-")
	if err != nil {
		return fmt.Errorf("無法寫入安裝位置：%w", err)
	}
	probe.Close()
	os.Remove(probe.Name())
	dir, err := os.MkdirTemp("", "YourDesk-update-")
	if err != nil {
		return err
	}
	handedOff := false
	defer func() {
		if !handedOff {
			os.RemoveAll(dir)
		}
	}()
	// 獨立副本避免下載檔案或安裝目錄在退出後被覆寫。
	installer := filepath.Join(dir, "setup.exe")
	source, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer source.Close()
	output, err := os.OpenFile(installer, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, source)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	config, _ := json.Marshal(map[string]any{"parent": os.Getpid(), "target": target})
	if err = os.WriteFile(filepath.Join(dir, "config.json"), config, 0600); err != nil {
		return err
	}
	script := filepath.Join(dir, "install.ps1")
	if err = os.WriteFile(script, []byte("\xef\xbb\xbf"+windowsUpdateScript), 0600); err != nil {
		return err
	}
	shell := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	cmd := exec.Command(shell, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script)
	handedOff = true
	return startUpdateHelper(ctx, cmd, dir)
}

const windowsUpdateScript = `
$ErrorActionPreference = 'Stop'
$dir = $PSScriptRoot
$backup = Join-Path $dir 'previous'
$installed = $false
try {
 $config = Get-Content -LiteralPath (Join-Path $dir 'config.json') -Raw -Encoding UTF8 | ConvertFrom-Json
 $parent = Get-Process -Id $config.parent -ErrorAction Stop
 Write-Output 'Updater ready; waiting for YourDesk to exit.'
 Set-Content -LiteralPath (Join-Path $dir 'ready') -Value 'ready'
 if (-not $parent.WaitForExit(120000)) { throw 'YourDesk did not exit within two minutes.' }
 Write-Output 'YourDesk exited; checking installation files.'
 # 其他執行個體仍在使用時停止更新，不終止不屬於本次 APP 的程序。
 $deadline = [DateTime]::UtcNow.AddSeconds(30)
 while ($true) {
  try {
   foreach ($name in @('YourDesk.exe','yourdesk-client.exe','yourdesk-remote.exe')) {
    $file = Join-Path $config.target $name
    if (Test-Path -LiteralPath $file) { $stream = [System.IO.File]::Open($file,'Open','ReadWrite','None'); $stream.Dispose() }
   }
   break
  } catch {
   if ([DateTime]::UtcNow -ge $deadline) { throw }
   Start-Sleep -Milliseconds 500
  }
 }
 New-Item -ItemType Directory -Path $backup | Out-Null
 foreach ($name in @('YourDesk.exe','yourdesk-client.exe','yourdesk-remote.exe','ThirdPartyLicenses','README.txt','使用說明.txt','Uninstall.exe')) {
  $file = Join-Path $config.target $name
  if (Test-Path -LiteralPath $file) { Copy-Item -LiteralPath $file -Destination $backup -Recurse }
 }
 $installed = $true
 # NSIS 的 /D 必須最後傳入，且即使路徑有空白也不能再加引號。
 Write-Output 'Starting silent installer.'
 $setup = Start-Process -FilePath (Join-Path $dir 'setup.exe') -ArgumentList ('/S /D=' + $config.target) -PassThru -Wait
 if ($setup.ExitCode -ne 0) { throw ('Installer exit code: ' + $setup.ExitCode) }
 Write-Output ('Installer finished: ' + $setup.ExitCode)
 $app = Join-Path $config.target 'YourDesk.exe'
 if (-not (Test-Path -LiteralPath $app)) { throw 'Updated application was not found.' }
 Remove-Item Env:YOURDESK_TEST_UPDATE -ErrorAction SilentlyContinue
 Start-Process -FilePath $app -WorkingDirectory $config.target
 Remove-Item -LiteralPath $dir -Recurse -Force -ErrorAction SilentlyContinue
} catch {
 $_ | Out-String | Add-Content -LiteralPath (Join-Path $dir 'update.log')
 if ($installed -and (Test-Path -LiteralPath $backup)) {
  try { Get-ChildItem -LiteralPath $backup | Copy-Item -Destination $config.target -Recurse -Force } catch { $_ | Out-String | Add-Content -LiteralPath (Join-Path $dir 'update.log') }
 }
 Add-Type -AssemblyName PresentationFramework
 [System.Windows.MessageBox]::Show(('YourDesk update failed. Please install the downloaded package manually. Log: ' + (Join-Path $dir 'update.log')), 'YourDesk') | Out-Null
 exit 1
}
`
