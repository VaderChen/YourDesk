package filetransfer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestListingUnicodeOrderMatchesLowercase(t *testing.T) {
	s, home := testSession(t)
	// The suffix keeps names distinct even on case-insensitive filesystems.
	stems := []string{"A", "a", "aA", "AA", "alpha", "ALPHA", "Z", "z", "É", "é", "İ", "i", "ı", "K", "k", "K", "Σ", "σ", "ς", "ẞ", "ß", "ſ", "s", "資料", "月報", "🎵", "🗂", "e\u0301", "𐐀", "𐐨"}
	var want []Entry
	for i, stem := range stems {
		for _, directory := range []bool{false, true} {
			name := fmt.Sprintf("%s-%03d-%t", stem, i, directory)
			p := filepath.Join(home, name)
			var err error
			if directory {
				err = os.Mkdir(p, 0700)
			} else {
				err = os.WriteFile(p, nil, 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			want = append(want, Entry{Name: name, Directory: directory})
		}
	}
	sort.Slice(want, func(i, j int) bool {
		a, b := want[i], want[j]
		if a.Directory != b.Directory {
			return a.Directory
		}
		left, right := strings.ToLower(a.Name), strings.ToLower(b.Name)
		if left != right {
			return left < right
		}
		return a.Name < b.Name
	})
	entries, err := sortedVisibleEntries(context.Background(), s.root, ".", 128)
	if err != nil {
		t.Fatal(err)
	}
	gotNames, wantNames := []string{}, []string{}
	for _, e := range entries {
		gotNames = append(gotNames, e.Name)
	}
	for _, e := range want {
		wantNames = append(wantNames, e.Name)
	}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("listing order changed:\n got %q\nwant %q", gotNames, wantNames)
	}
}

func BenchmarkVisibleDirectory(b *testing.B) {
	for _, count := range []int{256, 2048} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			home := b.TempDir()
			for i := 0; i < count; i++ {
				name := fmt.Sprintf("Report-%04d-Monthly-資料.txt", (i*137)%count)
				if err := os.WriteFile(filepath.Join(home, name), nil, 0600); err != nil {
					b.Fatal(err)
				}
			}
			root, err := os.OpenRoot(home)
			if err != nil {
				b.Fatal(err)
			}
			defer root.Close()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				entries, err := sortedVisibleEntries(context.Background(), root, ".", count)
				if err != nil || len(entries) != count {
					b.Fatalf("got %d entries, %v", len(entries), err)
				}
			}
		})
	}
}
