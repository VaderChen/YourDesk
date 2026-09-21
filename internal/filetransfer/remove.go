package filetransfer

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path"
	"time"
)

const (
	maxRemoveEntries = 4096
	maxRemoveDepth   = 32
)

type removeNode struct {
	name     string
	info     os.FileInfo
	children []*removeNode
}

type removeResult struct {
	OK           bool   `json:"ok"`
	RemovedCount int    `json:"removedCount"`
	Partial      bool   `json:"partial,omitempty"`
	Error        string `json:"error,omitempty"`
}

// remove preflights and validates a bounded tree before the first unlink. It
// does not call RemoveAll. Each deletion is anchored to a checked directory
// handle and rechecks identity/version; concurrent local changes can still
// interrupt a multi-entry operation, so partial results are explicit.
func (s *session) remove(ctx context.Context, raw json.RawMessage) (any, error) {
	var in struct {
		Path      string `json:"path"`
		Directory bool   `json:"directory"`
		Modified  string `json:"modified"`
		Size      int64  `json:"size"`
		Confirm   string `json:"confirm"`
		Recursive bool   `json:"recursive"`
	}
	if err := decode(raw, &in); err != nil {
		return nil, err
	}
	name, err := relative(in.Path)
	if err != nil {
		return nil, err
	}
	if name == "." || in.Confirm != path.Base(name) || in.Modified == "" || in.Size < 0 || (!in.Directory && in.Recursive) {
		return nil, errors.New("刪除須確認正確檔名、類型及目前版本；不可刪除使用者根目錄")
	}
	if s.conflictsWithUpload(name, in.Directory) {
		return nil, errors.New("此路徑包含尚未完成或暫停的上傳，請先取消傳輸")
	}
	parent, base, err := s.parent(name)
	if err != nil {
		return nil, err
	}
	defer parent.Close()
	info, err := parent.Lstat(base)
	if err != nil {
		return nil, err
	}
	if (!info.IsDir() && !info.Mode().IsRegular()) || info.IsDir() != in.Directory || info.Size() != in.Size || info.ModTime().UTC().Format(time.RFC3339Nano) != in.Modified {
		return nil, errors.New("檔案已變更或不是一般檔案／目錄，請重新整理後確認")
	}
	node := &removeNode{name: base, info: info}
	count := 0
	if err = s.preflightRemove(ctx, parent, node, name, 1, &count, in.Recursive); err != nil {
		return nil, err
	}
	if err = validateRemove(ctx, parent, node); err != nil {
		return nil, err
	}
	removed := 0
	if err = executeRemove(ctx, parent, node, &removed); err != nil {
		if removed == 0 {
			return nil, err
		}
		message := "刪除中斷：部分項目已永久刪除，其餘保留；請重新整理後確認"
		if ctx.Err() != nil {
			message = "刪除已取消或逾時：部分項目已永久刪除，其餘保留；請重新整理後確認"
		}
		return removeResult{OK: false, RemovedCount: removed, Partial: true, Error: message}, nil
	}
	return removeResult{OK: true, RemovedCount: removed}, nil
}

func (s *session) preflightRemove(ctx context.Context, parent *os.Root, node *removeNode, name string, depth int, count *int, recursive bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	*count++
	if *count > maxRemoveEntries || depth > maxRemoveDepth {
		return errors.New("刪除範圍超過 4096 個項目或 32 層目錄，請分批處理")
	}
	if _, err := relative(name); err != nil {
		return errors.New("刪除範圍包含受保護暫存或不支援的檔名")
	}
	if !node.info.IsDir() {
		if !node.info.Mode().IsRegular() {
			return errors.New("刪除範圍包含連結或特殊檔案")
		}
		return nil
	}
	r, err := openRemoveDir(parent, node)
	if err != nil {
		return err
	}
	defer r.Close()
	f, err := openDirectory(r, ".")
	if err != nil {
		return err
	}
	defer f.Close()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		entries, readErr := f.ReadDir(64)
		if len(entries) > 0 && !recursive {
			return errors.New("非空目錄必須明確選擇包含所有內容的遞迴刪除")
		}
		for _, entry := range entries {
			if *count >= maxRemoveEntries {
				return errors.New("刪除範圍超過 4096 個項目，請分批處理")
			}
			info, err := r.Lstat(entry.Name())
			if err != nil {
				return err
			}
			child := &removeNode{name: entry.Name(), info: info}
			if err = s.preflightRemove(ctx, r, child, path.Join(name, entry.Name()), depth+1, count, recursive); err != nil {
				return err
			}
			node.children = append(node.children, child)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	current, err := r.Stat(".")
	if err != nil {
		return err
	}
	if !sameVersion(current, node.info) {
		return errors.New("目錄內容在檢查時已變更，尚未刪除任何項目")
	}
	return nil
}

func openRemoveDir(parent *os.Root, node *removeNode) (*os.Root, error) {
	before, err := parent.Lstat(node.name)
	if err != nil {
		return nil, err
	}
	if !before.IsDir() || !sameVersion(before, node.info) {
		return nil, errors.New("待刪除目錄已變更")
	}
	r, err := parent.OpenRoot(node.name)
	if err != nil {
		return nil, err
	}
	opened, err := r.Stat(".")
	after, checkErr := parent.Lstat(node.name)
	if err != nil || checkErr != nil || !sameVersion(opened, node.info) || !sameVersion(after, node.info) {
		r.Close()
		return nil, errors.New("待刪除目錄已變更")
	}
	return r, nil
}

func checkRemoveChildren(ctx context.Context, r *os.Root, node *removeNode) error {
	f, err := openDirectory(r, ".")
	if err != nil {
		return err
	}
	defer f.Close()
	expected := make(map[string]bool, len(node.children))
	for _, child := range node.children {
		expected[child.name] = true
	}
	seen := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		entries, readErr := f.ReadDir(64)
		for _, entry := range entries {
			if !expected[entry.Name()] {
				return errors.New("待刪除目錄內容已變更")
			}
			seen++
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	if seen != len(node.children) {
		return errors.New("待刪除目錄內容已變更")
	}
	return nil
}

func validateRemove(ctx context.Context, parent *os.Root, node *removeNode) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := parent.Lstat(node.name)
	if err != nil {
		return err
	}
	if !sameVersion(info, node.info) {
		return errors.New("待刪除項目已變更，請重新整理後確認")
	}
	if !node.info.IsDir() {
		return nil
	}
	r, err := openRemoveDir(parent, node)
	if err != nil {
		return err
	}
	defer r.Close()
	if err = checkRemoveChildren(ctx, r, node); err != nil {
		return err
	}
	for _, child := range node.children {
		if err = validateRemove(ctx, r, child); err != nil {
			return err
		}
	}
	return nil
}

func executeRemove(ctx context.Context, parent *os.Root, node *removeNode, removed *int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := parent.Lstat(node.name)
	if err != nil {
		return err
	}
	if !sameVersion(info, node.info) {
		return errors.New("待刪除項目已變更，已停止刪除")
	}
	if node.info.IsDir() {
		r, err := openRemoveDir(parent, node)
		if err != nil {
			return err
		}
		if err = checkRemoveChildren(ctx, r, node); err != nil {
			r.Close()
			return err
		}
		for _, child := range node.children {
			if err = executeRemove(ctx, r, child, removed); err != nil {
				r.Close()
				return err
			}
		}
		// Our own child unlinks changed the directory timestamp. Identity must
		// still match; Remove refuses a directory with concurrently-added entries.
		current, err := parent.Lstat(node.name)
		r.Close()
		if err != nil {
			return err
		}
		if !current.IsDir() || !os.SameFile(current, node.info) {
			return errors.New("待刪除目錄已移動或替換，已停止刪除")
		}
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = parent.Remove(node.name); err != nil {
		return err
	}
	*removed++
	return nil
}
