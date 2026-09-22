package filetransfer

import "golang.org/x/sys/windows"

func systemFilesystem(home string) (*filesystemScope, error) {
	bits, err := windows.GetLogicalDrives()
	if err != nil {
		return nil, err
	}
	scope := &filesystemScope{roots: map[string]string{}}
	for i := 0; i < 26; i++ {
		if bits&(1<<i) == 0 {
			continue
		}
		key := string(rune('A' + i))
		scope.roots[key] = key + `:\`
		if scope.initial == "" || key == "C" {
			scope.initial = key
		}
	}
	return scope, nil
}
