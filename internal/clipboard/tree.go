package clipboard

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const maxTreeEntries = 4096

func validateTreeRoots(roots []fileEntry) error {
	if len(roots) == 0 || len(roots) > MaxFiles {
		return errors.New("目錄根項目數量無效")
	}
	names := map[string]bool{}
	for _, entry := range roots {
		key := strings.ToLower(entry.Name)
		if !safeName(entry.Name) || names[key] || entry.Size < 0 || entry.Size > MaxFileBytes || entry.Directory && entry.Size != 0 {
			return errors.New("目錄根項目清單無效")
		}
		names[key] = true
	}
	return nil
}
func safeTreePath(name string) bool {
	if len(name) > 1024 || strings.HasSuffix(name, "/") {
		return false
	}
	parts := strings.Split(name, "/")
	if len(parts) > 64 {
		return false
	}
	for _, part := range parts {
		if !safeName(part) {
			return false
		}
	}
	return true
}

type treeWriter struct {
	file      *os.File
	remaining int64
}

func (w *treeWriter) Write(data []byte) (int, error) {
	if int64(len(data)) > w.remaining {
		return 0, errors.New("目錄傳輸總量超過 2 GiB（含目錄資訊）")
	}
	n, err := w.file.Write(data)
	w.remaining -= int64(n)
	return n, err
}

type treeReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r treeReader) Read(data []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(data)
}

// 先建立不可變的傳輸快照，不持續開啟所有子檔案，也不將整個目錄載入記憶體。
func prepareTree(ctx context.Context, paths []string) (archive *os.File, roots []fileEntry, err error) {
	var sourceInfos []os.FileInfo
	for _, source := range paths {
		info, e := os.Lstat(source)
		if e != nil {
			return nil, nil, e
		}
		size := info.Size()
		if info.IsDir() {
			size = 0
		}
		sourceInfos = append(sourceInfos, info)
		roots = append(roots, fileEntry{Name: filepath.Base(filepath.Clean(source)), Size: size, Directory: info.IsDir()})
	}
	if err = validateTreeRoots(roots); err != nil {
		return nil, nil, err
	}
	archive, err = os.CreateTemp("", "yourdesk-clipboard-tree-*")
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if err != nil {
			_ = archive.Close()
			_ = os.Remove(archive.Name())
		}
	}()
	writer := tar.NewWriter(&treeWriter{file: archive, remaining: MaxFileBytes})
	count := 0
	names := map[string]bool{}
	for i, source := range paths {
		// OpenRoot 約束所有子項目的存取範圍，避免掃描期間目錄被換成外部連結。
		base := filepath.Dir(filepath.Clean(source))
		walkStart := filepath.Base(filepath.Clean(source))
		if roots[i].Directory {
			base = source
			walkStart = "."
		}
		root, e := os.OpenRoot(base)
		if e != nil {
			return archive, nil, e
		}
		actual, statErr := root.Stat(walkStart)
		if statErr != nil || !os.SameFile(sourceInfos[i], actual) {
			root.Close()
			return archive, nil, errors.New("複製來源已變更")
		}
		e = fs.WalkDir(root.FS(), walkStart, func(local string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if e := ctx.Err(); e != nil {
				return e
			}
			info, e := entry.Info()
			if e != nil {
				return e
			}
			if !info.IsDir() && !info.Mode().IsRegular() {
				return errors.New("目錄含符號連結或特殊檔案，不支援傳輸")
			}
			name := local
			if roots[i].Directory {
				name = roots[i].Name
				if local != "." {
					name += "/" + local
				}
			}
			if !safeTreePath(name) || names[strings.ToLower(name)] {
				return fmt.Errorf("目錄含不相容或重複路徑：%s", name)
			}
			names[strings.ToLower(name)] = true
			count++
			if count > maxTreeEntries {
				return errors.New("目錄項目超過 4096 筆")
			}
			header := &tar.Header{Name: name, Mode: 0600, Size: info.Size(), ModTime: info.ModTime(), Typeflag: tar.TypeReg}
			if info.IsDir() {
				header.Mode = 0700
				header.Size = 0
				header.Typeflag = tar.TypeDir
			}
			if local == walkStart && (info.IsDir() != roots[i].Directory || !info.IsDir() && info.Size() != roots[i].Size) {
				return errors.New("複製來源已變更")
			}
			if e := writer.WriteHeader(header); e != nil {
				return e
			}
			if info.IsDir() {
				return nil
			}
			f, e := root.Open(filepath.FromSlash(local))
			if e != nil {
				return e
			}
			defer f.Close()
			actual, e := f.Stat()
			if e != nil {
				return e
			}
			if !os.SameFile(info, actual) {
				return errors.New("複製來源已變更")
			}
			if _, e = io.CopyN(writer, treeReader{ctx: ctx, reader: f}, info.Size()); e != nil {
				return e
			}
			actual, e = f.Stat()
			if e != nil {
				return e
			}
			if actual.Size() != info.Size() || !actual.ModTime().Equal(info.ModTime()) {
				return errors.New("檔案在建立傳輸快照時已修改")
			}
			return nil
		})
		root.Close()
		if e != nil {
			return archive, nil, e
		}
	}
	if err = writer.Close(); err != nil {
		return archive, nil, err
	}
	_, err = archive.Seek(0, io.SeekStart)
	return archive, roots, err
}

func extractTree(reader io.Reader, destination string, roots []fileEntry) error {
	destinationRoot, err := os.OpenRoot(destination)
	if err != nil {
		return err
	}
	defer destinationRoot.Close()
	expected := map[string]fileEntry{}
	for _, entry := range roots {
		expected[entry.Name] = entry
	}
	seen := map[string]bool{}
	directories := map[string]bool{}
	found := map[string]bool{}
	archive := tar.NewReader(reader)
	count := 0
	var total int64
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := strings.TrimSuffix(header.Name, "/")
		if !safeTreePath(name) || seen[strings.ToLower(name)] {
			return errors.New("目錄封存含無效或重複路徑")
		}
		isDir := header.Typeflag == tar.TypeDir
		if !isDir && header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return errors.New("目錄封存含不支援的項目")
		}
		if header.Size < 0 || header.Size > MaxFileBytes || isDir && header.Size != 0 {
			return errors.New("目錄項目大小無效")
		}
		count++
		total += header.Size
		if count > maxTreeEntries || total > MaxFileBytes {
			return errors.New("目錄項目數量或總量超出限制")
		}
		parent := path.Dir(name)
		if parent == "." {
			root, ok := expected[name]
			if !ok || root.Directory != isDir || !isDir && root.Size != header.Size {
				return errors.New("目錄封存與根項目清單不符")
			}
			found[name] = true
		} else if !directories[parent] {
			return errors.New("目錄封存缺少父目錄")
		}
		target := filepath.FromSlash(name)
		if isDir {
			if err = destinationRoot.Mkdir(target, 0700); err != nil {
				return err
			}
			directories[name] = true
		} else {
			file, err := destinationRoot.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if err != nil {
				return err
			}
			_, copyErr := io.CopyN(file, archive, header.Size)
			syncErr := file.Sync()
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if syncErr != nil {
				return syncErr
			}
			if closeErr != nil {
				return closeErr
			}
		}
		seen[strings.ToLower(name)] = true
	}
	if len(found) != len(expected) {
		return errors.New("目錄封存缺少根項目")
	}
	return nil
}
