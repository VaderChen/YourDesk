package filetransfer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestMacMountedDiskLabelsAndFiltering(t *testing.T) {
	mount := func(path string, flags uint32) unix.Statfs_t {
		var v unix.Statfs_t
		copy(v.Mntonname[:], path)
		v.Flags = flags
		return v
	}
	roots, labels := macRootsFromMounts([]unix.Statfs_t{
		mount("/", 0), mount("/System/Volumes/Data", 0),
		mount("/Volumes/extSSD", 0), mount("/Volumes/My Code:工作", 0),
		mount("/Volumes/Recovery", unix.MNT_DONTBROWSE), mount("/Volumes/.hidden", 0),
	})
	if len(roots) != 3 || roots["root"] != "/" {
		t.Fatal(roots)
	}
	seen := map[string]bool{}
	for key, path := range roots {
		if _, err := relative(key); err != nil {
			t.Fatal(err)
		}
		if key != "root" && labels[key] != filepath.Base(path) {
			t.Fatal("磁碟名稱遺失", labels)
		}
		seen[labels[key]] = true
	}
	if !seen["extSSD"] || !seen["My Code:工作"] {
		t.Fatal(labels)
	}
}

func TestMacDiskRefreshAndRootProtection(t *testing.T) {
	s, _ := testSession(t)
	disk := t.TempDir()
	if err := os.WriteFile(filepath.Join(disk, "file.txt"), []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	present := true
	s.filesystem = &filesystemScope{singleRoot: true, mountedRoots: func() (map[string]string, map[string]string, error) {
		roots, labels := map[string]string{"root": "/"}, map[string]string{"root": "/"}
		if present {
			roots["volume-test"] = disk
			labels["volume-test"] = "My Code:工作"
		}
		return roots, labels, nil
	}}
	out := mustCall(t, s, "list", map[string]any{}).(listResult)
	if len(out.Entries) != 2 || out.Entries[1].DisplayName != "My Code:工作" || !out.Entries[1].Root {
		t.Fatal(out)
	}
	out = mustCall(t, s, "list", map[string]any{"path": "volume-test"}).(listResult)
	if len(out.Entries) != 1 || out.Location.Parent == nil || *out.Location.Parent != "" {
		t.Fatal(out)
	}
	if _, _, err := s.parent("volume-test"); err == nil {
		t.Fatal("磁碟根目錄不得修改")
	}
	present = false
	if _, err := s.openDir("volume-test"); err == nil {
		t.Fatal("拔除磁碟後仍能開啟遺留路徑")
	}
	out = mustCall(t, s, "list", map[string]any{}).(listResult)
	if len(out.Entries) != 1 {
		t.Fatal(out)
	}
	present = true
	out = mustCall(t, s, "list", map[string]any{}).(listResult)
	if len(out.Entries) != 2 {
		t.Fatal("重新掛載未列出", out)
	}
}

func TestMacMountedDisksSmoke(t *testing.T) {
	scope, err := systemFilesystem(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	out, err := scope.listRoots(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Entries) == 0 || out.Location == nil || !out.Location.Virtual {
		t.Fatal(out)
	}
	for _, item := range out.Entries {
		t.Logf("磁碟：%s，路徑：%s", item.DisplayName, scope.roots[item.Path])
	}
}
