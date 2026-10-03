package filetransfer

import (
	"context"
	"errors"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
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
		if order := compareFoldedNames(a.Name, b.Name); order != 0 {
			return order < 0
		}
		return a.Name < b.Name
	})
	return entries, ctx.Err()
}

// 與 strings.ToLower 後的 UTF-8 字典序一致，但排序時不反覆配置小寫副本。
// 有效 UTF-8 的位元組順序與 rune 順序相同；大小寫相同時仍由呼叫端比較原名。
func compareFoldedNames(a, b string) int {
	for len(a) > 0 && len(b) > 0 {
		left, right := rune(a[0]), rune(b[0])
		an, bn := 1, 1
		if left >= utf8.RuneSelf {
			left, an = utf8.DecodeRuneInString(a)
		}
		if right >= utf8.RuneSelf {
			right, bn = utf8.DecodeRuneInString(b)
		}
		left, right = unicode.ToLower(left), unicode.ToLower(right)
		if left < right {
			return -1
		}
		if left > right {
			return 1
		}
		a, b = a[an:], b[bn:]
	}
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return 0
}
