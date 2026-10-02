package clientui

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"debug/pe"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPortableExtraction(t *testing.T) {
	for _, scenario := range []string{"valid", "traversal", "duplicate", "tampered", "wrong-arch", "unmanaged"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			archive := filepath.Join(dir, "update.zip")
			out, _ := os.Create(archive)
			z := zip.NewWriter(out)
			b := make([]byte, 152)
			copy(b, "MZ")
			binary.LittleEndian.PutUint32(b[0x3c:], 128)
			copy(b[128:], "PE\x00\x00")
			machine := uint16(pe.IMAGE_FILE_MACHINE_AMD64)
			if scenario == "wrong-arch" {
				machine = pe.IMAGE_FILE_MACHINE_ARM64
			}
			binary.LittleEndian.PutUint16(b[132:], machine)
			files := map[string][]byte{"YourDesk.exe": b, "yourdesk-client.exe": b, "yourdesk-remote.exe": b, "avcodec-62.dll": b, "avutil-60.dll": b, "swscale-9.dll": b, "ThirdPartyLicenses/example.txt": []byte("license")}
			if scenario == "traversal" {
				files["../outside"] = b
			}
			if scenario == "unmanaged" {
				files["personal.txt"] = b
			}
			var sums strings.Builder
			for n, data := range files {
				fmt.Fprintf(&sums, "%x  %s\n", sha256.Sum256(data), n)
				w, _ := z.Create("YourDesk/" + n)
				w.Write(data)
			}
			manifest := sums.String()
			if scenario == "tampered" {
				manifest = strings.Replace(manifest, manifest[:64], strings.Repeat("0", 64), 1)
			}
			w, _ := z.Create("YourDesk/SHA256SUMS")
			w.Write([]byte(manifest))
			if scenario == "duplicate" {
				w, _ = z.Create("YourDesk/YOURDESK.EXE")
				w.Write(b)
			}
			z.Close()
			out.Close()
			target := filepath.Join(dir, "payload")
			_, err := extractPortableUpdate(context.Background(), archive, target, "amd64")
			if (err == nil) != (scenario == "valid") {
				t.Fatalf("unexpected result: %v", err)
			}
			if err != nil {
				if _, e := os.Stat(target); !os.IsNotExist(e) {
					t.Fatal("failed payload remains")
				}
			}
		})
	}
}
func TestPortableExistingPackageSmoke(t *testing.T) {
	archive := os.Getenv("YOURDESK_PORTABLE_SMOKE_ZIP")
	if archive == "" {
		t.Skip("需指定既有 ZIP")
	}
	if _, err := extractPortableUpdate(context.Background(), archive, filepath.Join(t.TempDir(), "payload"), "amd64"); err != nil {
		t.Fatal(err)
	}
}

func TestPortableApplyAndRollback(t *testing.T) {
	shell, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("需要 PowerShell 執行隔離替換與還原 Smoke")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	payload := filepath.Join(dir, "payload")
	backup := filepath.Join(dir, "previous")
	os.Mkdir(target, 0700)
	os.Mkdir(payload, 0700)
	os.WriteFile(filepath.Join(target, "YourDesk.exe"), []byte("old"), 0600)
	os.WriteFile(filepath.Join(target, "personal.txt"), []byte("keep"), 0600)
	os.WriteFile(filepath.Join(payload, "YourDesk.exe"), []byte("new"), 0600)
	os.WriteFile(filepath.Join(payload, "avcodec-62.dll"), []byte("new dll"), 0600)
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
	script := "$ErrorActionPreference='Stop'\n" + windowsRollbackScript + windowsPortableApplyScript + "\n$target=" + quote(target) + "; $dir=" + quote(dir) + "; $backup=" + quote(backup) + "\n" + `
$files=@('YourDesk.exe','avcodec-62.dll')
Backup-ManagedFiles $target $backup $files
Install-PortablePayload $dir $target $files
if ((Get-Content -LiteralPath (Join-Path $target 'YourDesk.exe') -Raw) -ne 'new') { throw 'Update failed' }
Restore-ManagedFiles $target $backup $files
if ((Get-Content -LiteralPath (Join-Path $target 'YourDesk.exe') -Raw) -ne 'old') { throw 'Restore failed' }
if (Test-Path -LiteralPath (Join-Path $target 'avcodec-62.dll')) { throw 'New DLL remained after rollback' }
if ((Get-Content -LiteralPath (Join-Path $target 'personal.txt') -Raw) -ne 'keep') { throw 'User data changed' }
$failed=$false
try { Install-PortablePayload $dir $target @('YourDesk.exe','missing.dll') } catch { $failed=$true }
if (-not $failed) { throw 'Missing payload was accepted' }
Restore-ManagedFiles $target $backup $files
if ((Get-Content -LiteralPath (Join-Path $target 'YourDesk.exe') -Raw) -ne 'old') { throw 'Partial failure rollback failed' }
`
	file := filepath.Join(dir, "smoke.ps1")
	os.WriteFile(file, []byte(script), 0600)
	if out, err := exec.Command(shell, "-NoProfile", "-NonInteractive", "-File", file).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
}
