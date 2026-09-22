package clientui

// 此腳本始終保留使用者身分。停止／替換／恢復受保護服務由 SYSTEM worker 完成。
// 不含 RunAs；服務更新失敗不可退回另一輪 UAC。
const windowsServiceUpdateScript = windowsRollbackScript + `
$ErrorActionPreference = 'Stop'
$dir = $PSScriptRoot
$backup = Join-Path $dir 'previous'
$installed = $false
$appOpened = $false
$begun = $false
function Request-ServiceUpdate($operation) {
 if ($config.serviceTicket -cnotmatch '^[0-9a-f]{32}$') { throw 'Invalid service update ticket.' }
 $pipe = New-Object IO.Pipes.NamedPipeClientStream('.', ('YourDeskPrelogin-update-' + $config.serviceTicket), [IO.Pipes.PipeDirection]::InOut, [IO.Pipes.PipeOptions]::Asynchronous)
 try {
  $pipe.Connect(5000)
  $encoding = New-Object Text.UTF8Encoding($false)
  $writer = New-Object IO.StreamWriter($pipe,$encoding,1024,$true)
  $reader = New-Object IO.StreamReader($pipe,$encoding,$false,1024,$true)
  try {
   $writer.WriteLine((@{operation=$operation} | ConvertTo-Json -Compress))
   $writer.Flush()
   $read = $reader.ReadLineAsync()
   if (-not $read.Wait(180000)) { throw 'Service update timed out.' }
   if (-not $read.Result) { throw 'Service update connection closed.' }
   $reply = $read.Result | ConvertFrom-Json
   if ($reply.error) { throw $reply.error }
   return $reply.phase
  } finally { $writer.Dispose(); $reader.Dispose() }
 } finally { $pipe.Dispose() }
}
try {
 $config = Get-Content -LiteralPath (Join-Path $dir 'config.json') -Raw -Encoding UTF8 | ConvertFrom-Json
 if ((Request-ServiceUpdate 'status') -ne 'ready') { throw 'Service update is not ready.' }
 $parent = Get-Process -Id $config.parent -ErrorAction Stop
 Set-Content -LiteralPath (Join-Path $dir 'ready') -Value 'ready'
 if (-not $parent.WaitForExit(120000)) { throw 'YourDesk did not exit within two minutes.' }
 if (Test-Path -LiteralPath (Join-Path $dir 'cancelled')) { throw 'Update cancelled.' }
 $begun = $true
 if ((Request-ServiceUpdate 'begin') -ne 'stopped') { throw 'Service did not stop for update.' }
 # 不終止其他 APP 執行個體；其檔案仍占用時回復服務並停止本次更新。
 $deadline = [DateTime]::UtcNow.AddSeconds(30)
 while ($true) {
  try {
   foreach ($name in @('YourDesk.exe','yourdesk-client.exe','yourdesk-remote.exe')) {
    $file = Join-Path $config.target $name
    if (Test-Path -LiteralPath $file) { $stream = [IO.File]::Open($file,'Open','ReadWrite','None'); $stream.Dispose() }
   }
   break
  } catch {
   if ([DateTime]::UtcNow -ge $deadline) { throw }
   Start-Sleep -Milliseconds 500
  }
 }
 Backup-ManagedFiles $config.target $backup $config.files
 $installed = $true
 # 仍以一般使用者安裝，確保 HKCU、捷徑及重新開啟的 APP 都屬於原帳號。
 $setupArgs = '/S /SERVICEUPDATE /D=' + $config.target
 $setup = Start-Process -FilePath (Join-Path $dir 'setup.exe') -ArgumentList $setupArgs -PassThru -Wait
 if ($setup.ExitCode -ne 0) { throw ('Installer exit code: ' + $setup.ExitCode) }
 $app = Join-Path $config.target 'YourDesk.exe'
 if (-not (Test-Path -LiteralPath $app)) { throw 'Updated application was not found.' }
 if ((Request-ServiceUpdate 'commit') -ne 'applied') { throw 'Updated service did not pass health checks.' }
 Remove-Item Env:YOURDESK_TEST_UPDATE -ErrorAction SilentlyContinue
 Start-Process -FilePath $app -WorkingDirectory $config.target -ErrorAction Stop
 $appOpened = $true
 if ((Request-ServiceUpdate 'finish') -ne 'complete') { throw 'Service update confirmation failed.' }
 Remove-Item -LiteralPath $dir -Recurse -Force -ErrorAction SilentlyContinue
} catch {
 $_ | Out-String | Add-Content -LiteralPath (Join-Path $dir 'update.log')
 # 開啟 APP 後不能覆寫正在使用的檔案；確認回覆遺失時先重試確認。
 if ($appOpened) {
  for ($attempt = 0; $attempt -lt 2; $attempt++) {
   try { if ((Request-ServiceUpdate 'finish') -eq 'complete') { exit 0 } } catch {}
  }
  exit 1
 }
 try { Request-ServiceUpdate 'rollback' | Out-Null } catch { $_ | Out-String | Add-Content -LiteralPath (Join-Path $dir 'update.log') }
 if ($installed -and (Test-Path -LiteralPath $backup)) {
  try { Restore-ManagedFiles $config.target $backup $config.files } catch { $_ | Out-String | Add-Content -LiteralPath (Join-Path $dir 'update.log') }
 }
 if (-not (Test-Path -LiteralPath (Join-Path $dir 'ready'))) { exit 1 }
 if ($begun) {
  try { Start-Process -FilePath (Join-Path $config.target 'YourDesk.exe') -WorkingDirectory $config.target -ErrorAction Stop } catch {}
 }
 Add-Type -AssemblyName PresentationFramework
 [System.Windows.MessageBox]::Show(('YourDesk update failed. The previous service was restored when possible. Log: ' + (Join-Path $dir 'update.log')), 'YourDesk') | Out-Null
 exit 1
}
`
