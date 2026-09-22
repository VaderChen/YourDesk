// Package serviceupdate 驗證官方 Windows 服務更新套件；不接受客戶端提供的雜湊或執行路徑。
package serviceupdate

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"debug/pe"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const MaxPackageSize = 512 << 20

var Files = []string{"yourdesk-client.exe", "avcodec-62.dll", "avutil-60.dll", "swscale-9.dll"}
var tagPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)
var versionPattern = regexp.MustCompile(`^v?(\d{1,8})\.(\d{2})\.(\d{4})(?: build |[-.]build[.-])(\d{4})$`)

func VersionKey(s string) string {
	p := versionPattern.FindStringSubmatch(strings.TrimSpace(s))
	if p == nil {
		return ""
	}
	return fmt.Sprintf("%08s%s%s%s", p[1], p[2], p[3], p[4])
}

type Asset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
}
type Release struct {
	Tag    string  `json:"tag_name"`
	Name   string  `json:"name"`
	Draft  bool    `json:"draft"`
	Assets []Asset `json:"assets"`
}
type Manifest struct {
	Version      string `json:"version"`
	Architecture string `json:"architecture"`
	Protocol     int    `json:"protocol"`
}

// Identity 僅解析固定儲存庫的安裝器 URL；之後由服務自行取得 Release 中的可信雜湊。
func Identity(raw, arch string) (tag, installer string, err error) {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", "", errors.New("更新來源不是官方 GitHub Release")
	}
	p := strings.Split(u.Path, "/")
	if len(p) != 7 || u.Path != path.Clean(u.Path) || strings.Join(p[:5], "/") != "/VaderChen/YourDesk/releases/download" || !tagPattern.MatchString(p[5]) {
		return "", "", errors.New("無效的 Release 路徑")
	}
	a := arch
	if a == "amd64" {
		a = "x64"
	}
	if (a != "x64" && a != "arm64") || !strings.HasPrefix(p[6], "YourDesk-") || !strings.HasSuffix(p[6], "-windows-"+a+"-setup.exe") || strings.ContainsAny(p[6], `\:`) {
		return "", "", errors.New("更新套件架構不符")
	}
	return p[5], p[6], nil
}

func Select(release Release, raw, arch, installed string) (Asset, string, error) {
	tag, installer, err := Identity(raw, arch)
	if err != nil {
		return Asset{}, "", err
	}
	version := release.Tag
	if VersionKey(version) == "" {
		version = release.Name
	}
	if release.Draft || release.Tag != tag || VersionKey(version) == "" || VersionKey(installed) == "" || VersionKey(version) < VersionKey(installed) {
		return Asset{}, "", errors.New("服務更新不接受草稿、未知版本或較舊版本")
	}
	name := strings.TrimSuffix(installer, "-setup.exe") + "-service.zip"
	var selected Asset
	installerFound := false
	for _, a := range release.Assets {
		if a.Name == installer && a.URL == raw {
			installerFound = true
		}
		if a.Name == name {
			if selected.Name != "" {
				return Asset{}, "", errors.New("服務更新套件重複")
			}
			selected = a
		}
	}
	wantURL := "https://github.com/VaderChen/YourDesk/releases/download/" + tag + "/" + name
	hash, e := hex.DecodeString(strings.TrimPrefix(selected.Digest, "sha256:"))
	if !installerFound || selected.URL != wantURL || selected.Size <= 0 || selected.Size > MaxPackageSize || !strings.HasPrefix(selected.Digest, "sha256:") || e != nil || len(hash) != sha256.Size {
		return Asset{}, "", errors.New("Release 缺少有效的服務套件或 GitHub SHA-256，請確認附件完整")
	}
	return selected, version, nil
}

func officialClient() *http.Client {
	return &http.Client{Timeout: 8 * time.Minute, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		host := r.URL.Hostname()
		if len(via) >= 5 || r.URL.Scheme != "https" || r.URL.User != nil || r.URL.Port() != "" || (host != "api.github.com" && host != "github.com" && !strings.HasSuffix(host, ".githubusercontent.com")) {
			return errors.New("拒絕非 GitHub 的更新重新導向")
		}
		return nil
	}}
}

// Prepare 的 dir 必須由 SYSTEM 建立且一般使用者不可寫。HTTP 及解壓失敗不會碰已安裝檔案。
func Prepare(ctx context.Context, raw, arch, installed, dir string) (string, error) {
	tag, _, err := Identity(raw, arch)
	if err != nil {
		return "", err
	}
	client := officialClient()
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/repos/VaderChen/YourDesk/releases/tags/"+tag, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "YourDesk-Service-Updater")
	response, err := client.Do(req)
	if err != nil {
		return "", err
	}
	var release Release
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return "", fmt.Errorf("GitHub Release 查詢失敗：%d", response.StatusCode)
	}
	err = json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&release)
	response.Body.Close()
	if err != nil {
		return "", err
	}
	asset, version, err := Select(release, raw, arch, installed)
	if err != nil {
		return "", err
	}
	req, err = http.NewRequestWithContext(ctx, "GET", asset.URL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "YourDesk-Service-Updater")
	response, err = client.Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("服務套件下載失敗：%d", response.StatusCode)
	}
	archive := filepath.Join(dir, "package.zip")
	if err = saveVerifiedPackage(response.Body, asset, archive); err != nil {
		return "", err
	}
	if err = Extract(archive, filepath.Join(dir, "next"), version, arch); err != nil {
		return "", err
	}
	_ = os.Remove(archive)
	return version, nil
}

func saveVerifiedPackage(source io.Reader, asset Asset, archive string) error {
	if asset.Size <= 0 || asset.Size > MaxPackageSize {
		return errors.New("服務套件大小不符")
	}
	file, err := os.OpenFile(archive, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	hash := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(source, asset.Size+1))
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if n != asset.Size || !strings.EqualFold("sha256:"+hex.EncodeToString(hash.Sum(nil)), asset.Digest) {
		return errors.New("服務套件 SHA-256 或大小不符")
	}
	return nil
}

func Extract(archive, destination, version, arch string) error {
	z, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer z.Close()
	if len(z.File) > 256 {
		return errors.New("服務套件項目過多")
	}
	wanted := map[string]bool{"manifest.json": true}
	for _, name := range Files {
		wanted[name] = true
	}
	seen := map[string]bool{}
	var size uint64
	for _, f := range z.File {
		if len(f.Name) > 1024 || f.Name != path.Clean(f.Name) || strings.ContainsAny(f.Name, `\:`) || strings.HasPrefix(f.Name, "/") || strings.HasPrefix(f.Name, "../") || !f.Mode().IsRegular() || seen[strings.ToLower(f.Name)] {
			return errors.New("服務套件含有不安全或重複項目")
		}
		seen[strings.ToLower(f.Name)] = true
		if !wanted[f.Name] && !strings.HasPrefix(f.Name, "ThirdPartyLicenses/") {
			return errors.New("服務套件含有未知項目")
		}
		if f.UncompressedSize64 > MaxPackageSize {
			return errors.New("服務套件過大")
		}
		size += f.UncompressedSize64
		if size > MaxPackageSize {
			return errors.New("服務套件過大")
		}
	}
	for name := range wanted {
		if !seen[name] {
			return fmt.Errorf("服務套件缺少 %s", name)
		}
	}
	if err = os.Mkdir(destination, 0700); err != nil {
		return err
	}
	for _, f := range z.File {
		if !wanted[f.Name] {
			continue
		}
		if f.Name == "manifest.json" && f.UncompressedSize64 > 4096 {
			return errors.New("套件資訊過大")
		}
		source, e := f.Open()
		if e != nil {
			return e
		}
		target, e := os.OpenFile(filepath.Join(destination, f.Name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			source.Close()
			return e
		}
		_, e = io.Copy(target, source)
		source.Close()
		ce := target.Close()
		if e != nil {
			return e
		}
		if ce != nil {
			return ce
		}
	}
	data, err := os.ReadFile(filepath.Join(destination, "manifest.json"))
	if err != nil {
		return err
	}
	var manifest Manifest
	if json.Unmarshal(data, &manifest) != nil || VersionKey(manifest.Version) == "" || VersionKey(manifest.Version) != VersionKey(version) || manifest.Architecture != arch || manifest.Protocol != 1 {
		return errors.New("服務套件版本、架構或協定不符")
	}
	machine := uint16(pe.IMAGE_FILE_MACHINE_AMD64)
	if arch == "arm64" {
		machine = pe.IMAGE_FILE_MACHINE_ARM64
	} else if arch != "amd64" {
		return errors.New("不支援的服務架構")
	}
	for _, name := range Files {
		binary, e := pe.Open(filepath.Join(destination, name))
		if e != nil {
			return e
		}
		correct := binary.Machine == machine
		binary.Close()
		if !correct {
			return errors.New("服務檔案 PE 架構不符")
		}
	}
	return nil
}
