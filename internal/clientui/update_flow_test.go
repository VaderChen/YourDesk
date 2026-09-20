package clientui

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type updateRoundTrip func(*http.Request) (*http.Response, error)

func (f updateRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAutomaticUpdateCapability(t *testing.T) {
	for _, tc := range []struct {
		system, name string
		want         bool
	}{
		{"windows", "YourDesk-windows-x64.zip", false},
		{"windows", "YourDesk-windows-x64-setup.exe", true},
		{"windows", "YourDesk-windows-arm64-setup.exe", true},
		{"darwin", "YourDesk-macos-arm64.dmg", true},
		{"darwin", "YourDesk-windows-x64.zip", false},
		{"linux", "YourDesk-linux.zip", false},
	} {
		if got := automaticUpdateSupported(tc.system, tc.name); got != tc.want {
			t.Errorf("%s/%s capability=%v", tc.system, tc.name, got)
		}
	}
}

func TestPortableDownloadOpensWithoutCountdownOrQuit(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	const content = "verified package fixture"
	oldTransport := http.DefaultTransport
	http.DefaultTransport = updateRoundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(content)), Request: r}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = oldTransport })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	u := newUpdateManager(ctx, t.TempDir())
	u.forceUpdate = true
	u.state = updateStatus{Available: true, Version: "test", Asset: releaseAsset{Name: "YourDesk-test-windows-x64.zip", URL: "https://github.com/VaderChen/YourDesk/releases/download/test/YourDesk-test-windows-x64.zip", Size: int64(len(content)), Digest: fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(content)))}}
	opened := make(chan string, 1)
	u.openPackage = func(ctx context.Context, path string) error {
		state := u.snapshot()
		if state.InstallAt != 0 || state.AutomaticInstall || !state.Opening {
			t.Errorf("Portable 狀態不符：%+v", state)
		}
		opened <- path
		return nil
	}
	var quit atomic.Bool
	if err := u.startDownload(func() { quit.Store(true) }); err != nil {
		t.Fatal(err)
	}
	select {
	case path := <-opened:
		if err := verifyDownloadedRelease(path, u.state.Asset); err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Portable 應直接開啟，不進入十秒倒數")
	}
	u.downloads.Wait()
	state := u.snapshot()
	if quit.Load() || state.InstallAt != 0 || state.Opening || state.DownloadError != "" || state.OpenError != "" {
		t.Fatalf("Portable 更新影響 APP 或未完成：%+v", state)
	}
}
