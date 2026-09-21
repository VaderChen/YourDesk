package clientui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileDragPaths(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "測試 file.txt")
	if err := os.WriteFile(file, []byte("complete"), 0600); err != nil {
		t.Fatal(err)
	}
	paths, err := validatedFileDragPaths([]string{file, file})
	if err != nil || len(paths) != 1 || paths[0] != file {
		t.Fatalf("file list = %v, %v", paths, err)
	}
	if paths, err = validatedFileDragPaths(nil); err != nil || len(paths) != 0 {
		t.Fatalf("empty list must disable drag: %v, %v", paths, err)
	}
	for _, invalid := range []string{"relative.txt", dir, filepath.Join(dir, "missing"), file + "\x00", file + "\xff"} {
		if _, err := validatedFileDragPaths([]string{invalid}); err == nil {
			t.Errorf("accepted invalid source %q", invalid)
		}
	}
	if _, err := validatedFileDragPaths(make([]string, 65)); err == nil {
		t.Fatal("accepted excessive file count")
	}
}

func TestFileDragRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "source")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(file, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := validatedFileDragPaths([]string{link}); err == nil {
		t.Fatal("accepted symbolic link")
	}
}
