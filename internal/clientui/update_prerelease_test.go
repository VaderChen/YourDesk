package clientui

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestPrereleaseSelectionAndIsolation(t *testing.T) {
	old := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = old })
	asset := releaseAsset{Name: "YourDesk-test-" + runtime.GOOS + "-" + runtime.GOARCH + ".zip", Size: 12, URL: "https://github.com/VaderChen/YourDesk/releases/download/test/package.zip"}
	if runtime.GOOS == "darwin" {
		asset.Name = "YourDesk-test-macos-" + runtime.GOARCH + ".dmg"
	}
	newer := githubRelease{Tag: "1.26.0919-build-0535", Prerelease: true, PublishedAt: time.Unix(200, 0), Assets: []releaseAsset{asset}}
	older := newer
	older.Tag = "1.26.0918-build-0001"
	older.PublishedAt = time.Unix(100, 0)
	draft := newer
	draft.Draft = true
	draft.PublishedAt = time.Unix(400, 0)
	stable := newer
	stable.Prerelease = false
	stable.PublishedAt = time.Unix(500, 0)
	calls := 0
	http.DefaultTransport = updateRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		var body any
		if strings.HasSuffix(r.URL.Path, "/latest") {
			body = stable
		} else if r.URL.Query().Get("page") == "1" {
			page := make([]githubRelease, 100)
			for i := range page {
				page[i] = stable
			}
			page[0] = older
			page[1] = draft
			body = page
		} else {
			body = []githubRelease{newer}
		}
		b, _ := json.Marshal(body)
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(b)))}, nil
	})
	got, err := fetchReleaseChannel(context.Background(), true)
	if err != nil || got.Version != newer.Tag || !got.Available || !got.Prerelease || got.Asset != asset || calls != 2 {
		t.Fatalf("selection: %+v, %v, calls=%d", got, err, calls)
	}
	got.CheckedAt = time.Now()
	u := &updateManager{state: got, forceUpdate: true, notify: make(chan struct{}, 1)}
	u.notifyPending()
	if len(u.notify) != 0 {
		t.Fatal("測試版本不得主動通知")
	}
	u.check(context.Background(), false)
	if calls != 2 {
		t.Fatal("背景檢查覆蓋手動選定測試版")
	}
	u.check(context.Background(), true, true)
	if u.snapshot().Prerelease || calls != 3 {
		t.Fatal("手動正式版檢查應切回正式版")
	}
	// 不開啟強制更新時，API 不得查詢測試版。
	s := &server{updater: u}
	w := httptest.NewRecorder()
	s.handleUpdates(w, httptest.NewRequest("GET", "/api/updates?prerelease=true", nil))
	if w.Code != 400 || calls != 3 {
		t.Fatal("缺少強制更新保護")
	}
}

func TestPrereleaseUnavailableClearsPreviousSelection(t *testing.T) {
	old := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = old })
	for _, code := range []int{200, 500} {
		http.DefaultTransport = updateRoundTrip(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader("[]")), Header: make(http.Header)}, nil
		})
		u := &updateManager{forceUpdate: true, state: updateStatus{Available: true, Version: "old", Asset: releaseAsset{Name: "old", URL: "old"}, DownloadPath: "old"}}
		got := u.checkChannel(context.Background(), true, true, true)
		if got.Available || got.Asset.URL != "" || got.DownloadPath != "" {
			t.Fatalf("stale selection: %+v", got)
		}
	}
}
