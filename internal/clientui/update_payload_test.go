package clientui

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestWindowsRollbackCoversInstallerPayload(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "scripts", "windows-installer.nsi"))
	if err != nil {
		t.Fatal(err)
	}
	managed := make(map[string]bool)
	for _, name := range windowsManagedUpdateFiles {
		if managed[name] || filepath.Base(name) != name || strings.ContainsAny(name, `/\:`) {
			t.Fatal("invalid managed name", name)
		}
		managed[name] = true
	}
	patterns := []*regexp.Regexp{
		// /x 可重複指定引號或未加引號的排除樣式；只擷取最後的安裝項目。
		// 其他未知選項仍交由下方未匹配檢查報錯，避免漏掉回復清單。
		regexp.MustCompile(`^File(?: /r)?(?: /x (?:"[^"\r\n]+"|[^\s"]+))* "\$\{PAYLOAD_DIR\}/([^"/]+)"$`),
		regexp.MustCompile(`^File /oname=([^ ]+) `),
		regexp.MustCompile(`^(?:Delete|WriteUninstaller) "\$INSTDIR\\([^"\\]+)"$`),
	}
	inSection := false
	seen := make(map[string]bool)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == `Section "YourDesk"` {
			inSection = true
			continue
		}
		if inSection && line == "SectionEnd" {
			break
		}
		if !inSection {
			continue
		}
		matched := false
		for _, re := range patterns {
			parts := re.FindStringSubmatch(line)
			if parts == nil {
				continue
			}
			matched = true
			seen[parts[1]] = true
			if !managed[parts[1]] {
				t.Errorf("NSIS 項目未納入備份／回復：%s", parts[1])
			}
		}
		if !matched && (strings.HasPrefix(line, "File ") || strings.HasPrefix(line, "Delete ") || strings.HasPrefix(line, "WriteUninstaller ")) {
			t.Errorf("請擴充發行清單對照，不可忽略新指令：%s", line)
		}
	}
	for name := range managed {
		if !seen[name] {
			t.Errorf("回復範圍包含非 NSIS 管理項目：%s", name)
		}
	}
}

func TestWindowsManagedRollbackRestoresDLLsAndRemovesNewFiles(t *testing.T) {
	shell := "powershell.exe"
	if runtime.GOOS != "windows" {
		shell = "pwsh"
	}
	shell, err := exec.LookPath(shell)
	if err != nil {
		t.Skip("需 Windows PowerShell 或 pwsh 才能執行隔離的實際 rollback 測試")
	}
	dir := t.TempDir()
	target, backup := filepath.Join(dir, "target"), filepath.Join(dir, "previous")
	if err := os.MkdirAll(filepath.Join(target, "ThirdPartyLicenses"), 0700); err != nil {
		t.Fatal(err)
	}
	before := map[string]string{"YourDesk.exe": "old exe", "avcodec-62.dll": "old codec", "ThirdPartyLicenses/old.txt": "old license", "personal.txt": "keep user file"}
	for name, content := range before {
		if err := os.WriteFile(filepath.Join(target, filepath.FromSlash(name)), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	config, _ := json.Marshal(map[string]any{"target": target, "backup": backup, "files": windowsManagedUpdateFiles})
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, config, 0600); err != nil {
		t.Fatal(err)
	}
	script := "$ErrorActionPreference = 'Stop'\n" + windowsRollbackScript + `
$c = Get-Content -LiteralPath '` + strings.ReplaceAll(configPath, "'", "''") + `' -Raw -Encoding UTF8 | ConvertFrom-Json
Backup-ManagedFiles $c.target $c.backup $c.files
Set-Content -LiteralPath (Join-Path $c.target 'YourDesk.exe') -Value 'new exe'
Set-Content -LiteralPath (Join-Path $c.target 'avcodec-62.dll') -Value 'broken new codec'
Set-Content -LiteralPath (Join-Path $c.target 'avutil-60.dll') -Value 'new dll not in old version'
Set-Content -LiteralPath (Join-Path $c.target 'ThirdPartyLicenses/new.txt') -Value 'new license'
Restore-ManagedFiles $c.target $c.backup $c.files
`
	scriptPath := filepath.Join(dir, "rollback-test.ps1")
	if err := os.WriteFile(scriptPath, []byte("\xef\xbb\xbf"+script), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(shell, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", scriptPath).CombinedOutput(); err != nil {
		t.Fatalf("rollback: %v\n%s", err, out)
	}
	for name, want := range before {
		got, err := os.ReadFile(filepath.Join(target, filepath.FromSlash(name)))
		if err != nil || string(got) != want {
			t.Errorf("%s 未完整還原：%q %v", name, got, err)
		}
	}
	for _, name := range []string{"avutil-60.dll", "ThirdPartyLicenses/new.txt"} {
		if _, err := os.Stat(filepath.Join(target, filepath.FromSlash(name))); !os.IsNotExist(err) {
			t.Errorf("新版項目殘留：%s %v", name, err)
		}
	}
}
