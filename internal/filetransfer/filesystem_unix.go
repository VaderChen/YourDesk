//go:build !windows

package filetransfer

import (
	"path/filepath"
	"strings"
)

func unixFilesystem(home string) (*filesystemScope, error) {
	resolved, err := filepath.EvalSymlinks(home)
	if err != nil {
		return nil, err
	}
	initial := "root/" + strings.TrimPrefix(filepath.ToSlash(resolved), "/")
	initial, err = relative(strings.TrimSuffix(initial, "/"))
	if err != nil {
		return nil, err
	}
	return &filesystemScope{roots: map[string]string{"root": "/"}, initial: initial, home: initial, singleRoot: true}, nil
}
