package clientui

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"debug/pe"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// 在 APP 退出前完成解壓、清單與架構驗證；僅寫入新的暫存目錄。
func extractPortableUpdate(ctx context.Context, archive, destination, arch string) ([]string, error) {
	z, err := zip.OpenReader(archive)
	if err != nil {
		return nil, err
	}
	defer z.Close()
	if len(z.File) > 10000 {
		return nil, errors.New("更新 ZIP 項目過多")
	}
	allowed := map[string]bool{}
	for _, n := range windowsManagedUpdateFiles {
		allowed[n] = true
	}
	delete(allowed, "YourDesk.installed")
	delete(allowed, "Uninstall.exe")
	delete(allowed, "使用說明.txt")
	allowed["SHA256SUMS"] = true
	if err = os.Mkdir(destination, 0700); err != nil {
		return nil, err
	}
	good := false
	defer func() {
		if !good {
			os.RemoveAll(destination)
		}
	}()
	hashes := map[string]string{}
	tops := map[string]bool{}
	seen := map[string]bool{}
	var total uint64
	for _, f := range z.File {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		name := strings.TrimPrefix(f.Name, "YourDesk/")
		if name == f.Name || name == "" || path.Clean(name) != name || strings.ContainsAny(name, `\:`) || strings.HasPrefix(name, "/") || !f.Mode().IsRegular() {
			return nil, errors.New("更新 ZIP 含不合法路徑或連結")
		}
		for _, c := range strings.Split(name, "/") {
			base := strings.ToUpper(strings.SplitN(c, ".", 2)[0])
			if c == ".." || strings.ContainsAny(c, "<>\"|?*\x00\r\n") || strings.TrimRight(c, ". ") != c || base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || (len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '0' && base[3] <= '9') {
				return nil, errors.New("更新 ZIP 檔名無效")
			}
		}
		top, _, nested := strings.Cut(name, "/")
		if !allowed[top] || (nested && top != "ThirdPartyLicenses") || seen[strings.ToLower(name)] {
			return nil, errors.New("更新 ZIP 含非預期或重複檔案")
		}
		seen[strings.ToLower(name)] = true
		if f.UncompressedSize64 > 1<<30 || total+f.UncompressedSize64 > 3<<30 {
			return nil, errors.New("更新 ZIP 解壓大小超出上限")
		}
		total += f.UncompressedSize64
		target := filepath.Join(destination, filepath.FromSlash(name))
		if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return nil, err
		}
		in, e := f.Open()
		if e != nil {
			return nil, e
		}
		out, e := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			in.Close()
			return nil, e
		}
		h := sha256.New()
		n, e := io.Copy(io.MultiWriter(out, h), io.LimitReader(in, int64(f.UncompressedSize64)+1))
		in.Close()
		ce := out.Close()
		if e != nil {
			return nil, e
		}
		if ce != nil {
			return nil, ce
		}
		if uint64(n) != f.UncompressedSize64 {
			return nil, errors.New("更新 ZIP 長度不符")
		}
		hashes[name] = hex.EncodeToString(h.Sum(nil))
		tops[top] = true
	}
	manifest, err := os.ReadFile(filepath.Join(destination, "SHA256SUMS"))
	if err != nil {
		return nil, err
	}
	verified := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(manifest)), "\n") {
		sum, name, ok := strings.Cut(line, "  ")
		if !ok || name == "SHA256SUMS" || verified[name] || hashes[name] != sum || len(sum) != 64 {
			return nil, errors.New("更新 ZIP 校驗清單不符")
		}
		verified[name] = true
	}
	if len(verified) != len(hashes)-1 {
		return nil, errors.New("更新 ZIP 校驗清單不完整")
	}
	for _, name := range []string{"YourDesk.exe", "yourdesk-client.exe", "yourdesk-remote.exe", "avcodec-62.dll", "avutil-60.dll", "swscale-9.dll"} {
		binary, e := pe.Open(filepath.Join(destination, name))
		if e != nil {
			return nil, e
		}
		machine := binary.Machine
		binary.Close()
		if (arch != "amd64" && arch != "arm64") || (arch == "amd64" && machine != pe.IMAGE_FILE_MACHINE_AMD64) || (arch == "arm64" && machine != pe.IMAGE_FILE_MACHINE_ARM64) {
			return nil, fmt.Errorf("更新套件架構不符：%s", name)
		}
	}
	files := []string{}
	for _, name := range windowsManagedUpdateFiles {
		if tops[name] {
			files = append(files, name)
		}
	}
	files = append(files, "SHA256SUMS")
	good = true
	return files, nil
}

const windowsPortableApplyScript = `
function Install-PortablePayload($dir, $target, $files) {
 $payload = Join-Path $dir 'payload'
 Assert-ManagedPath $payload
 foreach ($name in $files) { Assert-ManagedPath (Join-Path $target $name) }
 foreach ($name in $files) {
  $source = Join-Path $payload $name
  if (-not (Test-Path -LiteralPath $source)) { throw ('Missing portable file: ' + $name) }
  $dest = Join-Path $target $name
  if (Test-Path -LiteralPath $dest) { Remove-Item -LiteralPath $dest -Recurse -Force -ErrorAction Stop }
  Copy-Item -LiteralPath $source -Destination $dest -Recurse -Force -ErrorAction Stop
 }
}
`
