package serviceupdate

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"debug/pe"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fixtureURL = "https://github.com/VaderChen/YourDesk/releases/download/v1.26.0922-build.1200/YourDesk-1.26.0922-build-1200-windows-x64-setup.exe"
const fixtureVersion = "1.26.0922 build 1200"

func TestPackageHashAndSizeVerifiedBeforeExtraction(t *testing.T) {
	content := []byte("official package")
	asset := Asset{Size: int64(len(content)), Digest: fmt.Sprintf("sha256:%x", sha256.Sum256(content))}
	for name, input := range map[string][]byte{"valid": content, "tampered": []byte("untrusted bytes!"), "truncated": content[:3], "oversize": append(append([]byte{}, content...), 0)} {
		err := saveVerifiedPackage(bytes.NewReader(input), asset, filepath.Join(t.TempDir(), "package.zip"))
		if (err == nil) != (name == "valid") {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func fixtureRelease() Release {
	tag, name, _ := Identity(fixtureURL, "amd64")
	return Release{Tag: tag, Assets: []Asset{
		{Name: name, URL: fixtureURL, Size: 50},
		{Name: strings.TrimSuffix(name, "-setup.exe") + "-service.zip", URL: strings.TrimSuffix(fixtureURL, "-setup.exe") + "-service.zip", Size: 100, Digest: "sha256:" + strings.Repeat("ab", 32)},
	}}
}

func TestOfficialReleaseIdentityAndVersion(t *testing.T) {
	valid := fixtureRelease()
	if _, v, err := Select(valid, fixtureURL, "amd64", fixtureVersion); err != nil || VersionKey(v) != VersionKey(fixtureVersion) {
		t.Fatalf("官方同版重裝應可用：%q %v", v, err)
	}
	for _, raw := range []string{
		strings.Replace(fixtureURL, "github.com/", "github.com.evil/", 1),
		strings.Replace(fixtureURL, "github.com/", "evil@github.com/", 1),
		strings.Replace(fixtureURL, "https:", "http:", 1),
		strings.Replace(fixtureURL, "VaderChen/", "another/", 1),
		strings.Replace(fixtureURL, "github.com/", "github.com:443/", 1),
		strings.Replace(fixtureURL, "v1.26.0922-build.1200/", "../", 1),
		strings.Replace(fixtureURL, "v1.26.0922-build.1200/", "tag%2Fother/", 1),
		fixtureURL + "?download=1", fixtureURL + "#fragment",
	} {
		if _, _, err := Identity(raw, "amd64"); err == nil {
			t.Errorf("接受無效來源：%s", raw)
		}
	}
	if _, _, err := Identity(fixtureURL, "arm64"); err == nil {
		t.Fatal("接受錯誤架構")
	}
	cases := map[string]func(*Release){
		"draft":          func(r *Release) { r.Draft = true },
		"different-tag":  func(r *Release) { r.Tag = "v1.26.0922-build.1300" },
		"digest-missing": func(r *Release) { r.Assets[1].Digest = "" },
		"digest-short":   func(r *Release) { r.Assets[1].Digest = "sha256:00" },
		"digest-invalid": func(r *Release) { r.Assets[1].Digest = "sha256:" + strings.Repeat("z", 64) },
		"size":           func(r *Release) { r.Assets[1].Size = MaxPackageSize + 1 },
		"wrong-repo":     func(r *Release) { r.Assets[1].URL = strings.Replace(r.Assets[1].URL, "VaderChen/", "evil/", 1) },
		"no-installer":   func(r *Release) { r.Assets = r.Assets[1:] },
		"duplicate":      func(r *Release) { r.Assets = append(r.Assets, r.Assets[1]) },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			r := fixtureRelease()
			change(&r)
			if _, _, err := Select(r, fixtureURL, "amd64", fixtureVersion); err == nil {
				t.Fatal("接受不完整或不可信 Release")
			}
		})
	}
	for _, installed := range []string{"1.26.0922 build 1300", "unknown"} {
		if _, _, err := Select(valid, fixtureURL, "amd64", installed); err == nil {
			t.Fatalf("允許降版或未知版本 %s", installed)
		}
	}
}

func TestUpdateRedirectsStayOnGitHubHTTPS(t *testing.T) {
	for _, s := range []string{"http://github.com/a", "https://evil.test/a", "https://githubusercontent.com.evil/a", "https://release-assets.githubusercontent.com:8443/a", "https://x@y.githubusercontent.com/a"} {
		u, _ := url.Parse(s)
		if err := officialClient().CheckRedirect(&http.Request{URL: u}, nil); err == nil {
			t.Errorf("接受重新導向 %s", s)
		}
	}
	u, _ := url.Parse("https://release-assets.githubusercontent.com/a")
	if err := officialClient().CheckRedirect(&http.Request{URL: u}, nil); err != nil {
		t.Fatal(err)
	}
}

func fixturePE(machine uint16) []byte {
	b := make([]byte, 152)
	copy(b, "MZ")
	binary.LittleEndian.PutUint32(b[0x3c:], 128)
	copy(b[128:], "PE\x00\x00")
	binary.LittleEndian.PutUint16(b[132:], machine)
	return b
}

func fixtureEntries() map[string][]byte {
	m, _ := json.Marshal(Manifest{Version: fixtureVersion, Architecture: "amd64", Protocol: 1})
	result := map[string][]byte{"manifest.json": m, "ThirdPartyLicenses/example/LICENSE": []byte("fixture")}
	for _, name := range Files {
		result[name] = fixturePE(pe.IMAGE_FILE_MACHINE_AMD64)
	}
	return result
}

func writeZIP(t *testing.T, entries map[string][]byte, extra *zip.FileHeader) string {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, data := range entries {
		f, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if extra != nil {
		f, err := writer.CreateHeader(extra)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = f.Write([]byte("link"))
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(t.TempDir(), "package.zip")
	if err := os.WriteFile(name, buffer.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return name
}

func TestServiceZIPAllowsOnlyExpectedFiles(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "next")
	if err := Extract(writeZIP(t, fixtureEntries(), nil), dest, fixtureVersion, "amd64"); err != nil {
		t.Fatal(err)
	}
	items, err := os.ReadDir(dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != len(Files)+1 {
		t.Fatalf("不應抽出授權資料或其他執行檔：%v", items)
	}
	for name, change := range map[string]func(map[string][]byte){
		"traversal":        func(e map[string][]byte) { e["../escape"] = nil },
		"absolute":         func(e map[string][]byte) { e["/escape"] = nil },
		"ads":              func(e map[string][]byte) { e["yourdesk-client.exe:ads"] = nil },
		"backslash":        func(e map[string][]byte) { e[`..\escape`] = nil },
		"extra-executable": func(e map[string][]byte) { e["other.exe"] = nil },
		"case-duplicate":   func(e map[string][]byte) { e["YOURDESK-CLIENT.EXE"] = nil },
		"missing-dll":      func(e map[string][]byte) { delete(e, Files[1]) },
		"wrong-pe":         func(e map[string][]byte) { e[Files[0]] = fixturePE(pe.IMAGE_FILE_MACHINE_ARM64) },
		"invalid-pe":       func(e map[string][]byte) { e[Files[0]] = []byte("not PE") },
		"wrong-manifest": func(e map[string][]byte) {
			e["manifest.json"] = []byte(`{"version":"1.26.0922 build 1300","architecture":"amd64","protocol":1}`)
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := fixtureEntries()
			change(e)
			if err := Extract(writeZIP(t, e, nil), filepath.Join(t.TempDir(), "next"), fixtureVersion, "amd64"); err == nil {
				t.Fatal("接受不安全服務套件")
			}
		})
	}
	for _, header := range []*zip.FileHeader{{Name: "yourdesk-client.exe"}, {Name: "ThirdPartyLicenses/link"}} {
		if strings.Contains(header.Name, "link") {
			header.SetMode(os.ModeSymlink | 0777)
		}
		if err := Extract(writeZIP(t, fixtureEntries(), header), filepath.Join(t.TempDir(), "next"), fixtureVersion, "amd64"); err == nil {
			t.Fatal("接受重複檔案或符號連結")
		}
	}
}
