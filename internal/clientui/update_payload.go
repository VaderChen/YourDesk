package clientui

// 必須與 windows-installer.nsi 的 File／Delete／WriteUninstaller 一致。
// 僅管理這些項目，回復時不碰安裝目錄內使用者自行新增的其他檔案。
var windowsManagedUpdateFiles = []string{
	"YourDesk.exe", "YourDesk.installed", "yourdesk-client.exe", "yourdesk-remote.exe",
	"avcodec-62.dll", "avutil-60.dll", "swscale-9.dll",
	"release_zh-Hant.note", "release_en.note", "release_ja.note", "release_ko.note",
	"ThirdPartyLicenses", "README.txt", "使用說明.txt", "Uninstall.exe",
	"LICENSE.md", "LICENSE.en.md", "LICENSE.ja.md", "LICENSE.ko.md",
}

// 共用的 PowerShell 回復函數也供隔離的 Windows 測試呼叫；不執行安裝器。
const windowsRollbackScript = `
function Assert-ManagedPath($path) {
 if (Test-Path -LiteralPath $path) {
  $item = Get-Item -LiteralPath $path -Force
  if ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) { throw ('Refusing linked update path: ' + $path) }
  if ($item.PSIsContainer) {
   foreach ($child in @(Get-ChildItem -LiteralPath $path -Recurse -Force)) {
    if ($child.Attributes -band [IO.FileAttributes]::ReparsePoint) { throw ('Refusing linked update path: ' + $child.FullName) }
   }
  }
 }
}
function Backup-ManagedFiles($target, $backup, $files) {
 New-Item -ItemType Directory -Path $backup -ErrorAction Stop | Out-Null
 foreach ($name in $files) {
  $file = Join-Path $target $name
  Assert-ManagedPath $file
  if (Test-Path -LiteralPath $file) { Copy-Item -LiteralPath $file -Destination $backup -Recurse -Force -ErrorAction Stop }
 }
}
function Restore-ManagedFiles($target, $backup, $files) {
 # 完成所有路徑檢查再開始還原；備份保留在獨立暫存目錄。
 foreach ($name in $files) {
  Assert-ManagedPath (Join-Path $target $name)
  Assert-ManagedPath (Join-Path $backup $name)
 }
 foreach ($name in $files) {
  $file = Join-Path $target $name
  $saved = Join-Path $backup $name
  # 清除受管理的新項目，避免原本不存在的 DLL／授權檔殘留。
  if (Test-Path -LiteralPath $file) { Remove-Item -LiteralPath $file -Recurse -Force -ErrorAction Stop }
  if (Test-Path -LiteralPath $saved) { Copy-Item -LiteralPath $saved -Destination $target -Recurse -Force -ErrorAction Stop }
 }
}
`
