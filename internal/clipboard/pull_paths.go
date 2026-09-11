package clipboard

import (
	"path/filepath"
	"strings"
	"sync"
)

var pullMounts = struct {
	sync.RWMutex
	roots map[string]bool
}{roots: map[string]bool{}}

func isPullPath(name string) bool {
	pullMounts.RLock()
	defer pullMounts.RUnlock()
	for root := range pullMounts.roots {
		rel, e := filepath.Rel(root, name)
		if e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
