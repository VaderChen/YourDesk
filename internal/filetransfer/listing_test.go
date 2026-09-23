package filetransfer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestListingFoldersFirstAndHiddenAcrossPages(t *testing.T) {
	s, home := testSession(t)
	// 刻意以反向順序建立，資料夾數超過一頁，檔名又排在資料夾之前。
	want := []string{}
	for i := 19; i >= 0; i-- {
		if err := os.Mkdir(filepath.Join(home, fmt.Sprintf("Folder %02d", i)), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(home, fmt.Sprintf(".hidden-%02d", i)), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 20; i++ {
		want = append(want, fmt.Sprintf("Folder %02d", i))
	}
	for _, name := range []string{"Zulu.txt", "Bravo.txt", "alpha.txt"} {
		if err := os.WriteFile(filepath.Join(home, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(home, ".hidden-folder"), 0700); err != nil {
		t.Fatal(err)
	}
	want = append(want, "alpha.txt", "Bravo.txt", "Zulu.txt")
	var names []string
	for offset := 0; offset != -1; {
		out := mustCall(t, s, "list", map[string]any{"offset": offset}).(listResult)
		if len(out.Entries) == 0 || (out.NextOffset != -1 && out.NextOffset <= offset) {
			t.Fatalf("分頁未前進：%+v", out)
		}
		for _, item := range out.Entries {
			names = append(names, item.Name)
		}
		offset = out.NextOffset
	}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("清單順序或隱藏過濾不符：%v", names)
	}
	if out := mustCall(t, s, "list", map[string]any{"offset": 24}).(listResult); len(out.Entries) != 0 || out.NextOffset != -1 {
		t.Fatalf("清單結尾不符：%+v", out)
	}
}

func TestListingBoundAndCancellation(t *testing.T) {
	s, home := testSession(t)
	for _, name := range []string{"visible", ".hidden"} {
		if err := os.WriteFile(filepath.Join(home, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := sortedVisibleEntries(context.Background(), s.root, ".", 1); err == nil {
		t.Fatal("隱藏項目也必須計入掃描上限")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := sortedVisibleEntries(ctx, s.root, ".", 10); !errors.Is(err, context.Canceled) {
		t.Fatalf("未遵守取消：%v", err)
	}
}

// 名稱含空格或方括號的正常目錄／檔案，不得當成隱藏項目或無效路徑。
func TestListingOrdinaryNamesAcrossPages(t *testing.T) {
	s, home := testSession(t)
	want := map[string]bool{}
	for _, name := range []string{"projects", "My Code", "[projects]", "[My Code]"} {
		if err := os.Mkdir(filepath.Join(home, name), 0700); err != nil {
			t.Fatal(err)
		}
		want[name] = true
		if err := os.WriteFile(filepath.Join(home, name+".txt"), []byte("smoke"), 0600); err != nil {
			t.Fatal(err)
		}
		want[name+".txt"] = false
	}
	// 讓上述項目分布於多頁，確保不能把第一頁誤當成完整清單。
	for i := 0; i < 2*pageSize; i++ {
		if err := os.Mkdir(filepath.Join(home, fmt.Sprintf("Folder %02d", i)), 0700); err != nil {
			t.Fatal(err)
		}
	}
	seen := map[string]bool{}
	for offset := 0; offset != -1; {
		out := mustCall(t, s, "list", map[string]any{"offset": offset}).(listResult)
		for _, item := range out.Entries {
			if directory, ok := want[item.Name]; ok {
				if seen[item.Name] || item.Directory != directory {
					t.Fatalf("項目重複或類型錯誤：%+v", item)
				}
				seen[item.Name] = true
				if directory {
					mustCall(t, s, "list", map[string]any{"path": item.Path})
				} else {
					mustCall(t, s, "stat", map[string]any{"path": item.Path})
				}
			}
		}
		offset = out.NextOffset
	}
	if len(seen) != len(want) {
		t.Fatalf("正常名稱漏列：找到 %v，預期 %v", seen, want)
	}
}
