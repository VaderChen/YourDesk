package filetransfer

import (
	"context"
	"encoding/json"
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
	roots        map[string]string
	initial      string
	home         string
	singleRoot   bool
	labels       map[string]string
	mountedRoots func() (map[string]string, map[string]string, error)
}

type directoryLocation struct {
	Display string  `json:"display"`
	Parent  *string `json:"parent"`
	Virtual bool    `json:"virtual"`
}

func (scope *filesystemScope) refreshRoots() error {
	if scope.mountedRoots == nil {
		return nil
	}
	roots, labels, err := scope.mountedRoots()
	if err != nil {
		return err
	}
	scope.roots, scope.labels = roots, labels
	return nil
}

func (scope *filesystemScope) openRoot(name string) (*os.Root, string, error) {
	if err := scope.refreshRoots(); err != nil {
		return nil, "", err
	}
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
	if scope.singleRoot && rest == "" && scope.mountedRoots == nil {
		out.Parent = nil
	}
	return out
}

func (scope *filesystemScope) listRoots(ctx context.Context, offset int) (listResult, error) {
	out := listResult{Path: ".", Entries: []Entry{}, NextOffset: -1, Location: scope.location(".")}
	if err := scope.refreshRoots(); err != nil {
		return out, err
	}
	encodedBase, _ := json.Marshal(out)
	budget := len(encodedBase) + 32
	keys := make([]string, 0, len(scope.roots))
	for key := range scope.roots {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i] == "root" {
			return keys[j] != "root"
		}
		if keys[j] == "root" {
			return false
		}
		left, right := scope.labels[keys[i]], scope.labels[keys[j]]
		if left == "" {
			left = keys[i]
		}
		if right == "" {
			right = keys[j]
		}
		a, b := strings.ToLower(left), strings.ToLower(right)
		if a != b {
			return a < b
		}
		return keys[i] < keys[j]
	})
	for i := offset; i < len(keys); i++ {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		if len(out.Entries) == pageSize {
			out.NextOffset = i
			break
		}
		key := keys[i]
		item := Entry{Name: key, DisplayName: scope.labels[key], Path: key, Directory: true, Root: true}
		encoded, _ := json.Marshal(item)
		if budget+len(encoded) > 14000 {
			out.NextOffset = i
			break
		}
		out.Entries = append(out.Entries, item)
		budget += len(encoded) + 1
	}
	return out, nil
}
