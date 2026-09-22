package clientui

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"
	"yourdesk/internal/filetransfer"
)

type fileProgress struct {
	Name     string  `json:"name"`
	Received int64   `json:"received"`
	Total    int64   `json:"total"`
	State    string  `json:"state,omitempty"`
	Speed    float64 `json:"speed"`
	ETA      float64 `json:"eta,omitempty"`
	Sequence uint64  `json:"sequence,omitempty"`
}
type preparedFile struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	Path string `json:"-"`
}
type fileDownloadClient struct {
	mu                                            sync.RWMutex
	endpoint, token, session, instance, downloads string
	client                                        *http.Client
}

type fileDownloadCallError struct {
	message   string
	transient bool
}

func (e *fileDownloadCallError) Error() string { return e.message }

func transientFileDownloadError(err error) bool {
	var callErr *fileDownloadCallError
	return errors.As(err, &callErr) && callErr.transient
}

func (c *fileDownloadClient) rebind(address string) (string, error) {
	u, params, err := parseFilesAddress(address)
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if u.Scheme+"://"+u.Host+"/api/files" != c.endpoint || params.Get("token") != c.token || params.Get("session") != c.session {
		return "", errors.New("不允許更換檔案視窗的來源或站台")
	}
	c.instance = params.Get("instance")
	return c.instance, nil
}

func parseFilesAddress(address string) (*url.URL, url.Values, error) {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.User != nil || u.Path != "/files.html" || u.RawQuery != "" {
		return nil, nil, errors.New("檔案傳輸視窗位址無效")
	}
	params, err := url.ParseQuery(u.Fragment)
	if err != nil || len(params["token"]) != 1 || len(params["session"]) != 1 || len(params["instance"]) != 1 || params.Get("token") == "" || params.Get("session") == "" || params.Get("instance") == "" || len(address) > 16384 {
		return nil, nil, errors.New("檔案傳輸工作階段無效")
	}
	for name, values := range params {
		if len(values) != 1 || (name != "token" && name != "session" && name != "instance" && name != "name" && name != "language") {
			return nil, nil, errors.New("檔案傳輸工作階段參數無效")
		}
	}
	return u, params, nil
}

func (c *fileDownloadClient) call(ctx context.Context, action string, params, out any) error {
	c.mu.RLock()
	instance := c.instance
	c.mu.RUnlock()
	body, err := json.Marshal(map[string]any{"session": c.session, "instance": instance, "action": action, "params": params})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("X-YourDesk-Token", c.token)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return &fileDownloadCallError{"下載連線中斷；請重新連線後繼續", true}
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 16385))
	if err != nil {
		return &fileDownloadCallError{"檔案傳輸回應中斷", true}
	}
	if len(data) > 16384 {
		return &fileDownloadCallError{"檔案傳輸回應過大", false}
	}
	if res.StatusCode != http.StatusOK {
		var failure struct {
			Error string `json:"error"`
			Code  string `json:"code"`
		}
		message := "檔案傳輸失敗"
		if json.Unmarshal(data, &failure) == nil && failure.Error != "" {
			message = failure.Error
		}
		return &fileDownloadCallError{message, failure.Code == "files_session_unavailable" || failure.Code == "files_request_interrupted" || res.StatusCode >= 500 || res.StatusCode == 408 || res.StatusCode == 429}
	}
	return json.Unmarshal(data, out)
}

type fileDownloadMetadata struct {
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	Modified  string `json:"modified"`
	Directory bool   `json:"directory"`
}

func sameDownloadMetadata(a, b fileDownloadMetadata) bool {
	return !b.Directory && a.Name == b.Name && a.Size == b.Size && a.Modified == b.Modified
}

func (c *fileDownloadClient) controlledCall(ctx context.Context, control *fileDownloadControl, epoch uint64, action string, params, out any) error {
	if control == nil {
		return c.call(ctx, action, params, out)
	}
	request, finish, err := control.request(epoch)
	if err != nil {
		return err
	}
	err = c.call(request, action, params, out)
	if !finish() {
		return errFileDownloadSuspended
	}
	return err
}

// On resume, metadata is checked before requesting any more data. The original
// snapshot is never replaced by the new one: a changed source cannot be mixed
// with the bytes already written to the partial file.
func (c *fileDownloadClient) downloadCheckpoint(ctx context.Context, control *fileDownloadControl, path string, expected *fileDownloadMetadata, validated *uint64) (fileDownloadMetadata, uint64, error) {
	if control == nil && expected != nil && validated != nil {
		return *expected, 0, nil
	}
	for {
		epoch := uint64(0)
		var err error
		if control != nil {
			epoch, err = control.waitRunning()
			if err != nil {
				return fileDownloadMetadata{}, 0, err
			}
			if expected != nil && validated != nil && *validated == epoch {
				return *expected, epoch, nil
			}
		}
		var meta fileDownloadMetadata
		err = c.controlledCall(ctx, control, epoch, "stat", map[string]any{"path": path}, &meta)
		if errors.Is(err, errFileDownloadSuspended) {
			continue
		}
		if err != nil {
			if control != nil && transientFileDownloadError(err) {
				control.interrupt(&epoch)
				continue
			}
			return meta, epoch, err
		}
		if expected != nil && !sameDownloadMetadata(*expected, meta) {
			return meta, epoch, errors.New("來源檔案已變更，無法繼續原下載；請重新下載")
		}
		if validated != nil {
			*validated = epoch
		}
		return meta, epoch, nil
	}
}

func safeDownloadName(name string) bool {
	if name == "" || len(name) > 255 || !utf8.ValidString(name) || strings.TrimRight(name, ". ") != name || strings.ContainsAny(name, "/\\\x00<>:\"|?*") {
		return false
	}
	for _, ch := range name {
		if ch < 32 {
			return false
		}
	}
	stem := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	if stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" || (len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '1' && stem[3] <= '9') {
		return false
	}
	return name != "." && name != ".."
}

// 完成的下載直接保留於所選目錄；關閉視窗不刪除已完成檔案。
func (c *fileDownloadClient) prepare(ctx context.Context, path string, progress func(fileProgress)) (out preparedFile, err error) {
	return c.prepareWithControl(ctx, path, c.downloads, progress, nil)
}

func (c *fileDownloadClient) prepareControlled(control *fileDownloadControl, path string) (out preparedFile, err error) {
	return c.prepareControlledAt(control, path, c.downloads)
}

func (c *fileDownloadClient) prepareControlledAt(control *fileDownloadControl, path, directory string) (out preparedFile, err error) {
	defer func() { control.finish(err) }()
	return c.prepareWithControl(control.ctx, path, directory, nil, control)
}

func (c *fileDownloadClient) prepareWithControl(ctx context.Context, path, directory string, progress func(fileProgress), control *fileDownloadControl) (out preparedFile, err error) {
	defer func() {
		var pe *os.PathError
		var le *os.LinkError
		if errors.As(err, &pe) || errors.As(err, &le) {
			err = errors.New("無法儲存下載：請確認下載目錄權限與剩餘空間")
		}
	}()
	meta, validatedEpoch, err := c.downloadCheckpoint(ctx, control, path, nil, nil)
	if err != nil {
		return
	}
	if meta.Directory || !safeDownloadName(meta.Name) || meta.Size < 0 || meta.Size > 2<<30 || meta.Modified == "" {
		return out, errors.New("只能下載名稱有效且不超過 2 GiB 的一般檔案")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return out, errors.New("無法開啟所選下載目錄")
	}
	defer root.Close()
	name := meta.Name
	if _, err := root.Lstat(name); err == nil {
		return out, errors.New("下載檔名已存在，不會覆寫既有檔案")
	} else if !errors.Is(err, os.ErrNotExist) {
		return out, err
	}
	partial := ".yourdesk-part-" + rand.Text()
	f, err := root.OpenFile(partial, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return out, err
	}
	original, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return out, err
	}
	ownedPartial := func() bool {
		current, err := root.Lstat(partial)
		return err == nil && current.Mode().IsRegular() && os.SameFile(original, current)
	}
	checkPartial := func(offset int64) error {
		current, err := f.Stat()
		if err != nil {
			return err
		}
		if !ownedPartial() || !current.Mode().IsRegular() || current.Size() != offset {
			return errors.New("本機暫存檔已變更；請重新下載")
		}
		return nil
	}
	complete := false
	defer func() {
		_ = f.Close()
		if !complete {
			if ownedPartial() {
				_ = root.Remove(partial)
			}
		}
	}()
	emitProgress := func(received int64) {
		if control != nil {
			control.update(meta.Name, received, meta.Size)
		}
		if progress != nil {
			progress(fileProgress{Name: meta.Name, Received: received, Total: meta.Size, State: "running"})
		}
	}
	emitProgress(0)
	var offset int64
	for offset < meta.Size {
		if err = ctx.Err(); err != nil {
			return out, err
		}
		previousEpoch := validatedEpoch
		_, epoch, checkpointErr := c.downloadCheckpoint(ctx, control, path, &meta, &validatedEpoch)
		if checkpointErr != nil {
			return out, checkpointErr
		}
		if epoch != previousEpoch {
			if err = checkPartial(offset); err != nil {
				return out, err
			}
		}
		var chunk struct {
			Data       string `json:"data"`
			Bytes      int    `json:"bytes"`
			NextOffset int64  `json:"nextOffset"`
			EOF        bool   `json:"eof"`
		}
		length := min(int64(4096), meta.Size-offset)
		if err = c.controlledCall(ctx, control, epoch, "read", map[string]any{"path": path, "offset": offset, "length": length, "modified": meta.Modified, "size": meta.Size}, &chunk); err != nil {
			if errors.Is(err, errFileDownloadSuspended) {
				continue
			}
			if control != nil && transientFileDownloadError(err) {
				control.interrupt(&epoch)
				continue
			}
			return out, err
		}
		data, e := base64.StdEncoding.DecodeString(chunk.Data)
		if e != nil || int64(len(data)) != length || chunk.Bytes != len(data) || chunk.NextOffset != offset+length || chunk.EOF != (offset+length == meta.Size) {
			return out, errors.New("遠端檔案區塊不完整或已變更")
		}
		if _, err = f.Write(data); err != nil {
			return out, err
		}
		offset += length
		emitProgress(offset)
	}
	// 包含零位元檔，提交前再驗證快照，以免完成後才發現遠端來源已變更。
	if _, _, err = c.downloadCheckpoint(ctx, control, path, &meta, nil); err != nil {
		return out, err
	}
	if err = ctx.Err(); err != nil {
		return out, err
	}
	if err = checkPartial(offset); err != nil {
		return out, err
	}
	if err = f.Sync(); err != nil {
		return out, err
	}
	if err = f.Close(); err != nil {
		return out, err
	}
	// 使用與上傳相同的不覆寫提交，不要求目的磁碟支援硬連結。
	if !ownedPartial() {
		return out, errors.New("本機暫存檔已被替換；請重新下載")
	}
	if err = filetransfer.PublishFile(root, partial, name, original); err != nil {
		return out, errors.New("無法完成下載：目的檔案已存在或目錄無法寫入")
	}
	published, err := root.Lstat(name)
	if err != nil || !published.Mode().IsRegular() || !os.SameFile(original, published) {
		return out, errors.New("下載檔案提交失敗；本機檔案已變更")
	}
	if ownedPartial() {
		_ = root.Remove(partial)
	}
	complete = true
	return preparedFile{Name: meta.Name, Size: meta.Size, Path: filepath.Join(directory, name)}, nil
}
