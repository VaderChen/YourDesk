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
func prepareAutomaticUpdate(ctx context.Context, archive, assetURL string) error {
	serviceEnabled := prelogin.Status().Enabled
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
	ticket := ""
	legacy := false
	if serviceEnabled {
		ticket, legacy, err = prelogin.PrepareServiceUpdate(ctx, assetURL)
		if err != nil {
			return err
		}
	}
	helperReady := false
	defer func() {
		if ticket != "" && !helperReady {
			prelogin.CancelServiceUpdate(context.Background(), ticket)
		}
	}()
	config, _ := json.Marshal(map[string]any{"parent": os.Getpid(), "target": target, "files": windowsManagedUpdateFiles, "prelogin": serviceEnabled, "serviceTicket": ticket})
	if err = os.WriteFile(filepath.Join(dir, "config.json"), config, 0600); err != nil {
		return err
	}
	script := filepath.Join(dir, "install.ps1")
	content := windowsUpdateScript
	if ticket != "" {
		content = windowsServiceUpdateScript
	}
	if err = os.WriteFile(script, []byte("\xef\xbb\xbf"+content), 0600); err != nil {
		return err
	}
	shell := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	cmd := exec.Command(shell, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script)
	handedOff = true
	err = startUpdateHelper(ctx, cmd, dir, legacy)
	helperReady = err == nil
	return err
}

const windowsUpdateScript = `param([switch]$Elevated, [switch]$Rollback)
` + windowsRollbackScript + `
$ErrorActionPreference = 'Stop'
$dir = $PSScriptRoot
$backup = Join-Path $dir 'previous'
$installed = $false
$serviceStopped = $false
$serviceSaved = $false
$serviceRoot = Join-Path ([Environment]::GetFolderPath('ProgramFiles')) 'YourDeskPrelogin'
$serviceBackup = Join-Path $dir 'previous-service'
$serviceFiles = @('yourdesk-client.exe','avcodec-62.dll','avutil-60.dll','swscale-9.dll')
$serviceManagedFiles = @($serviceFiles) + @('updater.protocol')
function Start-Prelogin {
 Start-Service YourDeskPrelogin
 (Get-Service YourDeskPrelogin).WaitForStatus('Running',[TimeSpan]::FromSeconds(30))
}
function Stop-Prelogin {
 Stop-Service YourDeskPrelogin
 (Get-Service YourDeskPrelogin).WaitForStatus('Stopped',[TimeSpan]::FromSeconds(30))
}
try {
 $config = Get-Content -LiteralPath (Join-Path $dir 'config.json') -Raw -Encoding UTF8 | ConvertFrom-Json
 if ($config.prelogin -and -not $Elevated) {
  $config | Add-Member -NotePropertyName updateUser -NotePropertyValue ([Security.Principal.WindowsIdentity]::GetCurrent().User.Value) -Force
  $config | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $dir 'config.json') -Encoding UTF8
  $shell = Join-Path $PSHOME 'powershell.exe'
  $arguments = '-NoProfile -NonInteractive -ExecutionPolicy Bypass -File "' + $PSCommandPath + '" -Elevated'
  $worker = Start-Process -FilePath $shell -Verb RunAs -ArgumentList $arguments -Wait -PassThru
  if ($worker.ExitCode -ne 0) { throw 'Service update was cancelled or failed.' }
  try {
   Remove-Item Env:YOURDESK_TEST_UPDATE -ErrorAction SilentlyContinue
   Start-Process -FilePath (Join-Path $config.target 'YourDesk.exe') -WorkingDirectory $config.target -ErrorAction Stop
  } catch {
   Start-Process -FilePath $shell -Verb RunAs -ArgumentList ($arguments + ' -Rollback') -Wait | Out-Null
   throw
  }
  Remove-Item -LiteralPath $dir -Recurse -Force -ErrorAction SilentlyContinue
  exit 0
 }
 if ($config.prelogin) {
  if (-not ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'Administrator authorization required.' }
  if ($config.updateUser -ne [Security.Principal.WindowsIdentity]::GetCurrent().User.Value) { throw 'Please run the update from an administrator account; installing under a different account is not supported.' }
  Assert-ManagedPath $serviceRoot
  $svc = Get-CimInstance Win32_Service -Filter "Name='YourDeskPrelogin'"
  $expected = '"' + (Join-Path $serviceRoot 'yourdesk-client.exe') + '" --prelogin daemon'
  if ($null -eq $svc -or $svc.PathName -ne $expected -or $svc.StartName -ne 'LocalSystem') { throw 'Unexpected pre-login service configuration.' }
  if ($Rollback) {
   $serviceStopped = $true
   $serviceSaved = $true
   $installed = $true
   Stop-Prelogin
   Restore-ManagedFiles $config.target $backup $config.files
   Restore-ManagedFiles $serviceRoot $serviceBackup $serviceManagedFiles
   Start-Prelogin
   exit 0
  }
 }
 $parent = Get-Process -Id $config.parent -ErrorAction Stop
 Write-Output 'Updater ready; waiting for YourDesk to exit.'
 Set-Content -LiteralPath (Join-Path $dir 'ready') -Value 'ready'
 if (-not $parent.WaitForExit(120000)) { throw 'YourDesk did not exit within two minutes.' }
 if (Test-Path -LiteralPath (Join-Path $dir 'cancelled')) { throw 'Update cancelled.' }
 Write-Output 'YourDesk exited; checking installation files.'
 if ($config.prelogin) {
  $serviceStopped = $true
  Stop-Prelogin
  Backup-ManagedFiles $serviceRoot $serviceBackup $serviceManagedFiles
  $serviceSaved = $true
 }
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
 Backup-ManagedFiles $config.target $backup $config.files
 $installed = $true
 # NSIS 的 /D 必須最後傳入，且即使路徑有空白也不能再加引號。
 Write-Output 'Starting silent installer.'
 $setupArgs = '/S /D=' + $config.target
 if ($config.prelogin) { $setupArgs = '/S /SERVICEUPDATE /D=' + $config.target }
 $setup = Start-Process -FilePath (Join-Path $dir 'setup.exe') -ArgumentList $setupArgs -PassThru -Wait
 if ($setup.ExitCode -ne 0) { throw ('Installer exit code: ' + $setup.ExitCode) }
 Write-Output ('Installer finished: ' + $setup.ExitCode)
 $app = Join-Path $config.target 'YourDesk.exe'
 if (-not (Test-Path -LiteralPath $app)) { throw 'Updated application was not found.' }
 if ($config.prelogin) {
  foreach ($name in $serviceFiles) {
   $source = Join-Path $config.target $name
   Assert-ManagedPath $source
   if (-not (Test-Path -LiteralPath $source)) { throw ('Missing service file: ' + $name) }
   Copy-Item -LiteralPath $source -Destination (Join-Path $serviceRoot $name) -Force
  }
  Start-Prelogin
  exit 0
 }
 Remove-Item Env:YOURDESK_TEST_UPDATE -ErrorAction SilentlyContinue
 Start-Process -FilePath $app -WorkingDirectory $config.target
 Remove-Item -LiteralPath $dir -Recurse -Force -ErrorAction SilentlyContinue
} catch {
 $_ | Out-String | Add-Content -LiteralPath (Join-Path $dir 'update.log')
 if ($serviceStopped) { try { Stop-Prelogin } catch {} }
 if ($installed -and (Test-Path -LiteralPath $backup)) {
  try { Restore-ManagedFiles $config.target $backup $config.files } catch { $_ | Out-String | Add-Content -LiteralPath (Join-Path $dir 'update.log') }
 }
 if ($serviceSaved) {
  try { Restore-ManagedFiles $serviceRoot $serviceBackup $serviceManagedFiles } catch { $_ | Out-String | Add-Content -LiteralPath (Join-Path $dir 'update.log') }
 }
 if ($serviceStopped) { try { Start-Prelogin } catch { $_ | Out-String | Add-Content -LiteralPath (Join-Path $dir 'update.log') } }
 if ($Elevated -or -not (Test-Path -LiteralPath (Join-Path $dir 'ready'))) { exit 1 }
 Add-Type -AssemblyName PresentationFramework
 [System.Windows.MessageBox]::Show(('YourDesk update failed. Please install the downloaded package manually. Log: ' + (Join-Path $dir 'update.log')), 'YourDesk') | Out-Null
 exit 1
}
`
