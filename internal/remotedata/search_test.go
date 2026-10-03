package remotedata

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestSearchContentBoundsAndIsolation(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "child"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{
		"first.txt":  strings.Repeat("a", 65530) + "needle" + "ignored",
		"beyond.txt": strings.Repeat("a", 65536) + "needle",
		"binary.txt": "needle\x00", "invalid.txt": "needle\xff",
		"short.txt": "tiny", "empty.txt": "", "child/hit.txt": "needle",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	wantSkipped := 1
	if err := os.Symlink(filepath.Join(dir, "first.txt"), filepath.Join(dir, "link.txt")); err != nil {
		if runtime.GOOS != "windows" {
			t.Fatal(err)
		}
		t.Logf("Windows symlink privilege unavailable; other search checks still run: %v", err)
		wantSkipped = 0
	}
	result, err := search(context.Background(), dir, Search{Query: ".TXT", Content: "needle"})
	if err != nil {
		t.Fatal(err)
	}
	out := result.(map[string]any)
	var paths []string
	for _, entry := range out["entries"].([]Entry) {
		path, _ := filepath.Rel(dir, entry.Path)
		paths = append(paths, filepath.ToSlash(path))
	}
	slices.Sort(paths)
	if !slices.Equal(paths, []string{"child/hit.txt", "first.txt"}) || out["truncated"] != true || out["skipped"] != wantSkipped {
		t.Fatalf("search leaked stale/binary/out-of-bound content: %#v", out)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := search(ctx, dir, Search{Content: "needle"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled search: %v", err)
	}
}

func TestReadSearchContentGrowthAndReuse(t *testing.T) {
	var scratch []byte
	for _, size := range []int{1, 70000, 17, 0, 65536, 512} {
		data := bytes.Repeat([]byte{'x'}, size)
		// A deliberately stale size hint must not truncate a growing file.
		result, err := readSearchContent(bytes.NewReader(data), scratch, 0)
		if err != nil || !bytes.Equal(result, data[:min(size, 65536)]) || cap(result) > 65536 {
			t.Fatalf("size=%d len=%d cap=%d err=%v", size, len(result), cap(result), err)
		}
		scratch = result
	}
	failure := errors.New("read failure")
	if _, err := readSearchContent(io.MultiReader(strings.NewReader("partial"), failingReader{failure}), scratch, 0); !errors.Is(err, failure) {
		t.Fatalf("read failure hidden: %v", err)
	}
}

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

func BenchmarkSearchContent(b *testing.B) {
	for _, size := range []int{128, 65536} {
		b.Run(fmt.Sprintf("100Files_%dB", size), func(b *testing.B) {
			dir := b.TempDir()
			data := bytes.Repeat([]byte{'x'}, size)
			for i := 0; i < 100; i++ {
				if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("file-%03d.txt", i)), data, 0600); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportAllocs()
			for b.Loop() {
				result, err := search(context.Background(), dir, Search{Content: "missing"})
				if err != nil || result.(map[string]any)["visited"] != 100 {
					b.Fatal(result, err)
				}
			}
		})
	}
}
