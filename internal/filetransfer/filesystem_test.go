package filetransfer

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func filesystemFixture(t *testing.T, hub *transferHub) (*session, string, *filesystemScope) {
	t.Helper()
	base := t.TempDir()
	home := filepath.Join(base, "C", "Users", "demo")
	for _, p := range []string{home, filepath.Join(base, "D")} {
		if err := os.MkdirAll(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	s, err := newSessionWithHub(home, func() bool { return true }, hub)
	if err != nil {
		t.Fatal(err)
	}
	s.filesystem = &filesystemScope{roots: map[string]string{"C": filepath.Join(base, "C"), "D": filepath.Join(base, "D")}, initial: "C"}
	t.Cleanup(s.close)
	return s, home, s.filesystem
}

func TestFilesystemNavigationAndTransfer(t *testing.T) {
	s, _, scope := filesystemFixture(t, newTransferHub())
	if got := mustCall(t, s, "location", map[string]any{}).(map[string]string)["path"]; got != "C" {
		t.Fatal(got)
	}
	for _, name := range []string{"C/Users/demo", "C/Users", "C", ""} {
		out := mustCall(t, s, "list", map[string]any{"path": name}).(listResult)
		if out.Location == nil {
			t.Fatal("缺少目錄位置")
		}
		if name == "" {
			if out.Location.Parent != nil || !out.Location.Virtual || len(out.Entries) != 2 || !out.Entries[0].Root {
				t.Fatal(out)
			}
		} else if out.Location.Parent == nil || *out.Location.Parent != strings.Join(strings.Split(name, "/")[:len(strings.Split(name, "/"))-1], "/") {
			t.Fatal(out)
		}
	}
	mustCall(t, s, "mkdir", map[string]string{"path": "D/傳輸"})
	id := startUpload(t, s, "D/傳輸/文件.txt", 3)
	writeChunk(t, s, id, 0, []byte("abc"))
	mustCall(t, s, "commit", map[string]string{"id": id})
	data, err := os.ReadFile(filepath.Join(scope.roots["D"], "傳輸", "文件.txt"))
	if err != nil || string(data) != "abc" {
		t.Fatalf("%q %v", data, err)
	}
	info := mustCall(t, s, "stat", map[string]string{"path": "D/傳輸/文件.txt"}).(Entry)
	mustCall(t, s, "read", map[string]any{"path": info.Path, "size": info.Size, "modified": info.Modified, "length": 3})
	for _, name := range []string{"", "C", "D", "C/../D/file", "/etc/passwd", "Z/file"} {
		if _, err := call(s, "mkdir", map[string]string{"path": name}); err == nil {
			t.Fatalf("接受根目錄／無效路徑：%q", name)
		}
	}
	s.allowed = func() bool { return false }
	if _, err := call(s, "list", map[string]string{"path": "C"}); err == nil {
		t.Fatal("失去授權仍可列目錄")
	}
}

func TestSystemFilesystemDefault(t *testing.T) {
	home := t.TempDir()
	scope, err := systemFilesystem(home)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if _, ok := scope.roots["C"]; ok && scope.initial != "C" {
			t.Fatal("Windows 初始位置不是 C 磁碟")
		}
		return
	}
	s, err := newSession(home, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	s.filesystem = scope
	defer s.close()
	out := mustCall(t, s, "list", map[string]string{"path": scope.initial}).(listResult)
	if len(out.Entries) != 0 || out.Location.Display != "~/" || out.Location.Parent == nil {
		t.Fatal("沒有以使用者家目錄作為起點", out)
	}
	root, err := s.openDir(scope.initial)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	got, err := root.Stat(".")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.Stat(home)
	if err != nil || !os.SameFile(got, want) {
		t.Fatal("初始位置不是已指定家目錄", err)
	}
}

func TestFilesystemResumeCannotCrossHomeOrScope(t *testing.T) {
	hub := newTransferHub()
	first, home, scope := filesystemFixture(t, hub)
	token := strings.Repeat("a", 64)
	args := map[string]any{"path": "D/file", "size": 3, "token": token}
	mustCall(t, first, "begin", args)
	first.detach()
	resume := map[string]any{"path": "D/file", "size": 3, "token": token, "id": token[:32]}
	for _, base := range []string{home, t.TempDir()} {
		other, err := newSessionWithHub(base, func() bool { return true }, hub)
		if err != nil {
			t.Fatal(err)
		}
		if base != home {
			other.filesystem = scope
		}
		for _, method := range []string{"resume", "cancel"} {
			if _, err := call(other, method, resume); err == nil {
				t.Fatalf("%s 跨帳號或路徑範圍認領", method)
			}
		}
		other.close()
	}
	second, err := newSessionWithHub(home, func() bool { return true }, hub)
	if err != nil {
		t.Fatal(err)
	}
	second.filesystem = scope
	t.Cleanup(second.close)
	mustCall(t, second, "resume", resume)
	writeChunk(t, second, token[:32], 0, []byte("abc"))
	mustCall(t, second, "commit", map[string]string{"id": token[:32]})
}

func TestFilesystemUnixHomeAndRoot(t *testing.T) {
	s, _, scope := filesystemFixture(t, newTransferHub())
	scope.roots = map[string]string{"root": scope.roots["C"]}
	scope.initial = "root/Users/demo"
	scope.home = scope.initial
	scope.singleRoot = true
	out := mustCall(t, s, "list", map[string]string{"path": scope.initial}).(listResult)
	if out.Location.Display != "~/" || out.Location.Parent == nil || *out.Location.Parent != "root/Users" {
		t.Fatal(out)
	}
	out = mustCall(t, s, "list", map[string]string{"path": "root"}).(listResult)
	if out.Location.Parent != nil {
		t.Fatal("實際根目錄仍有上一層")
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(scope.roots["root"], "link")); err != nil {
		t.Skip(err)
	}
	if _, err := call(s, "list", map[string]string{"path": "root/link"}); err == nil {
		t.Fatal("不應跟隨目錄連結")
	}
}
