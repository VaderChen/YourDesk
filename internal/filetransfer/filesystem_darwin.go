package filetransfer

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

func systemFilesystem(home string) (*filesystemScope, error) {
	scope, err := unixFilesystem(home)
	if err != nil {
		return nil, err
	}
	scope.mountedRoots = macMountedRoots
	if err := scope.refreshRoots(); err != nil {
		return nil, err
	}
	return scope, nil
}

// 直接查掛載表，不依賴被系統標為 hidden 的 /Volumes 目錄。
// 重新整理與存取時重新確認掛載狀態，拔除後不可寫入遺留的空目錄。
func macMountedRoots() (map[string]string, map[string]string, error) {
	for attempt := 0; attempt < 3; attempt++ {
		n, err := unix.Getfsstat(nil, unix.MNT_NOWAIT)
		if err != nil {
			return nil, nil, err
		}
		if n > 4096 {
			return nil, nil, errors.New("掛載磁碟數量超出上限")
		}
		mounts := make([]unix.Statfs_t, n+8)
		n, err = unix.Getfsstat(mounts, unix.MNT_NOWAIT)
		if err != nil {
			return nil, nil, err
		}
		if n >= len(mounts) {
			continue
		}
		roots, labels := macRootsFromMounts(mounts[:n])
		return roots, labels, nil
	}
	return nil, nil, errors.New("掛載磁碟正在變更，請重新整理")
}

func macRootsFromMounts(mounts []unix.Statfs_t) (map[string]string, map[string]string) {
	roots, labels := map[string]string{"root": "/"}, map[string]string{"root": "/"}
	for _, mount := range mounts {
		directory := unix.ByteSliceToString(mount.Mntonname[:])
		if filepath.Dir(directory) != "/Volumes" || mount.Flags&unix.MNT_DONTBROWSE != 0 {
			continue
		}
		name := filepath.Base(directory)
		if name == "." || name == ".." || strings.HasPrefix(name, ".") {
			continue
		}
		// 磁碟標籤與通訊路徑分開，合法的 Mac 磁碟名稱不受 Windows 檔名規則限制。
		digest := sha256.Sum256([]byte(directory))
		key := "volume-" + hex.EncodeToString(digest[:12])
		roots[key], labels[key] = directory, name
	}
	return roots, labels
}
