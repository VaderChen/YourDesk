package filetransfer

import (
	"context"
	"errors"
	"io"
	"os"
	"path"
	"sort"
	"strings"
)

// 分頁前先過濾及排序，避免下一頁的資料夾出現在前一頁檔案之後。
// 列舉量仍有上限，且每個項目都沿用 os.Root 的 Lstat 與路徑檢查。
func sortedVisibleEntries(ctx context.Context, root *os.Root, name string, limit int) ([]Entry, error) {
	f, err := openDirectory(root, ".")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	entries := []Entry{}
	visited := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		items, readErr := f.ReadDir(128)
		for _, item := range items {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			visited++
			if visited > limit {
				return nil, errors.New("目錄項目過多，超出排序上限")
			}
			if strings.HasPrefix(item.Name(), ".") {
				continue
			}
			p := path.Join(name, item.Name())
			if _, err := relative(p); err != nil {
				continue
			}
			info, err := root.Lstat(item.Name())
			if err != nil || (!info.IsDir() && !info.Mode().IsRegular()) || hiddenAttributes(info) {
				continue
			}
			entries = append(entries, entry(p, info))
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.Directory != b.Directory {
			return a.Directory
		}
		left, right := strings.ToLower(a.Name), strings.ToLower(b.Name)
		if left != right {
			return left < right
		}
		return a.Name < b.Name
	})
	return entries, ctx.Err()
}
