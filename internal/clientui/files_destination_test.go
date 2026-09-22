package clientui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFileDestinationDefaultsAndNavigation(t *testing.T) {
	home := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	} else {
		t.Setenv("HOME", home)
	}
	initial, err := localDirectoryPath("")
	want := home
	if runtime.GOOS == "windows" {
		want = `C:\`
	}
	if err != nil || initial != want {
		t.Fatalf("初始目錄：%q，預期 %q：%v", initial, want, err)
	}
	if path, err := localDirectoryPath("~/下載"); err != nil || path != filepath.Join(home, "下載") {
		t.Fatalf("家目錄展開失敗：%q：%v", path, err)
	}
	for _, name := range []string{"Alpha", "資料夾"} {
		if err := os.Mkdir(filepath.Join(home, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	file := filepath.Join(home, "不是目錄.txt")
	if err := os.WriteFile(file, []byte("保留"), 0600); err != nil {
		t.Fatal(err)
	}
	listing, err := listLocalDirectories(context.Background(), "~", 0)
	if err != nil || listing.Path != home || listing.Parent != filepath.Dir(home) || len(listing.Entries) != 2 || listing.NextOffset != -1 {
		t.Fatalf("本機目錄列舉錯誤：%+v：%v", listing, err)
	}
	for _, entry := range listing.Entries {
		if entry.Path != filepath.Join(home, entry.Name) || entry.Name == "不是目錄.txt" {
			t.Fatalf("目錄項目無效：%+v", entry)
		}
	}
	for _, path := range []string{"relative", "../parent", file, filepath.Join(home, "不存在")} {
		if _, err := listLocalDirectories(context.Background(), path, 0); err == nil {
			t.Fatalf("接受無效目錄：%q", path)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := listLocalDirectories(ctx, home, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("未取消目錄列舉：%v", err)
	}
}

func TestFileDestinationPagination(t *testing.T) {
	directory := t.TempDir()
	for i := range 270 {
		if err := os.Mkdir(filepath.Join(directory, fmt.Sprintf("資料夾-%03d", i)), 0700); err != nil {
			t.Fatal(err)
		}
	}
	seen := map[string]bool{}
	for offset := 0; offset >= 0; {
		listing, err := listLocalDirectories(context.Background(), directory, offset)
		if err != nil || len(listing.Entries) > 256 || (listing.NextOffset != -1 && listing.NextOffset <= offset) {
			t.Fatalf("分頁失敗：%+v：%v", listing, err)
		}
		for _, entry := range listing.Entries {
			if seen[entry.Name] {
				t.Fatalf("重複項目：%s", entry.Name)
			}
			seen[entry.Name] = true
		}
		offset = listing.NextOffset
	}
	if len(seen) != 270 {
		t.Fatalf("漏列目錄：%d", len(seen))
	}
}

func TestFileDownloadSelectedDestination(t *testing.T) {
	client := downloadFixture(t, []byte("直接下載至所選資料夾"), "")
	directory := t.TempDir()
	out, err := client.prepareWithControl(context.Background(), "sample", directory, nil, nil)
	if err != nil || out.Path != filepath.Join(directory, "範例 👋.txt") {
		t.Fatalf("下載目的地錯誤：%+v：%v", out, err)
	}
	data, err := os.ReadFile(out.Path)
	if err != nil || string(data) != "直接下載至所選資料夾" {
		t.Fatalf("下載內容錯誤：%q：%v", data, err)
	}
	if entries, err := os.ReadDir(client.downloads); err != nil || len(entries) != 0 {
		t.Fatalf("不應寫入預設目錄：%v：%v", entries, err)
	}
	if _, err := client.prepareWithControl(context.Background(), "sample", directory, nil, nil); err == nil {
		t.Fatal("不應覆寫同名檔案")
	}
	if entries, err := os.ReadDir(directory); err != nil || len(entries) != 1 {
		t.Fatalf("下載不應新增子目錄或殘留暫存：%v：%v", entries, err)
	}
}
