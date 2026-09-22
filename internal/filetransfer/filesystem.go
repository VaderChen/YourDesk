package filetransfer

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// 網路路徑仍使用相對路徑；第一段是伺服器指定的磁碟識別碼。
// 每次存取重新開啟該磁碟，再沿用 os.Root 的逐層檢查，不接受任意本機根路徑。
type filesystemScope struct {
	roots      map[string]string
	initial    string
	home       string
	singleRoot bool
}

type directoryLocation struct {
	Display string  `json:"display"`
	Parent  *string `json:"parent"`
	Virtual bool    `json:"virtual"`
}

func (scope *filesystemScope) openRoot(name string) (*os.Root, string, error) {
	clean, err := relative(name)
	if err != nil {
		return nil, "", err
	}
	key, rest, _ := strings.Cut(clean, "/")
	directory, ok := scope.roots[key]
	if !ok {
		return nil, "", errors.New("磁碟不存在或無法存取")
	}
	r, err := os.OpenRoot(directory)
	if err != nil {
		return nil, "", err
	}
	if rest == "" {
		rest = "."
	}
	return r, rest, nil
}

func (scope *filesystemScope) location(name string) *directoryLocation {
	if name == "." {
		return &directoryLocation{Virtual: true}
	}
	key, rest, _ := strings.Cut(name, "/")
	display := filepath.Join(scope.roots[key], filepath.FromSlash(rest))
	if scope.singleRoot && name == scope.home {
		display = "~/"
	}
	parent := path.Dir(name)
	if parent == "." {
		parent = ""
	}
	out := &directoryLocation{Display: display, Parent: &parent}
	if scope.singleRoot && rest == "" {
		out.Parent = nil
	}
	return out
}

func (scope *filesystemScope) listRoots(ctx context.Context, offset int) (listResult, error) {
	out := listResult{Path: ".", Entries: []Entry{}, NextOffset: -1, Location: scope.location(".")}
	keys := make([]string, 0, len(scope.roots))
	for key := range scope.roots {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for i := offset; i < len(keys); i++ {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		if len(out.Entries) == pageSize {
			out.NextOffset = i
			break
		}
		key := keys[i]
		out.Entries = append(out.Entries, Entry{Name: key, Path: key, Directory: true, Root: true})
	}
	return out, nil
}
