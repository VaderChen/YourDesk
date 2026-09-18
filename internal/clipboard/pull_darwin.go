//go:build darwin && cgo

package clipboard

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"yourdesk/internal/authlog"
)

func nativePullSupported() bool { _, err := os.Stat("/sbin/mount_webdav"); return err == nil }

type davReader struct {
	offer    *pullOffer
	index    int
	position int64
}

func (r *davReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.position >= r.offer.entries[r.index].Size {
		return 0, io.EOF
	}
	// Finder 所需的小段完成即可交付，不先等待整個 256 KiB 預讀區塊。
	// 保持單一同步請求與既有傳輸背壓，避免預取增加串流壅塞。
	if len(p) > 32*1024 {
		p = p[:32*1024]
	}
	n, err := r.offer.read(r.index, r.position, p)
	r.position += int64(n)
	return n, err
}

// 交付資料後最多每 100 ms 刷出 HTTP 緩衝；不另開 goroutine 操作 writer。
// 不提供 ReaderFrom，確保 ServeContent 的複製經過 Write。
type davProgressWriter struct {
	http.ResponseWriter
	lastFlush time.Time
}

func (w *davProgressWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if n > 0 && err == nil && time.Since(w.lastFlush) >= 100*time.Millisecond {
		if f, ok := w.ResponseWriter.(http.Flusher); ok {
			f.Flush()
			w.lastFlush = time.Now()
		}
	}
	return n, err
}
func (r *davReader) Seek(off int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		off += r.position
	case io.SeekEnd:
		off += r.offer.entries[r.index].Size
	default:
		return 0, os.ErrInvalid
	}
	if off < 0 {
		return 0, os.ErrInvalid
	}
	r.position = off
	return off, nil
}

type davResourceType struct {
	Collection *struct{} `xml:"D:collection,omitempty"`
}
type davProperties struct {
	Name     string          `xml:"D:displayname"`
	Type     davResourceType `xml:"D:resourcetype"`
	Size     int64           `xml:"D:getcontentlength"`
	Modified string          `xml:"D:getlastmodified"`
	ETag     string          `xml:"D:getetag"`
}
type davPropStat struct {
	Prop   davProperties `xml:"D:prop"`
	Status string        `xml:"D:status"`
}
type davResponse struct {
	Href string      `xml:"D:href"`
	Stat davPropStat `xml:"D:propstat"`
}
type davResponses struct {
	XMLName   xml.Name      `xml:"D:multistatus"`
	NS        string        `xml:"xmlns:D,attr"`
	Responses []davResponse `xml:"D:response"`
}

func nativePublishOffer(o *pullOffer) error {
	permissionError := errors.New("無法存取檔案貼上所需的網路卷宗；請在「系統設定 → 隱私權與安全性 → 檔案與檔案夾」允許 YourDesk 存取網路卷宗，然後重新複製檔案")
	if o.s.pull.permissionDeniedOffer == o.id {
		return permissionError
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	mount, err := os.MkdirTemp("", "yourdesk-paste-")
	if err != nil {
		listener.Close()
		return err
	}
	prefix := "/" + pullID() + "/"
	created := time.Now().UTC().Truncate(time.Second)
	entries := map[string]int{}
	for i, e := range o.entries {
		entries[e.Name] = i
	}
	// 每份清單每種方法只記錄一次，避免 Finder 中繼資料輪詢造成日誌洪水。
	var observedMethods atomic.Uint32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, prefix) {
			http.NotFound(w, r)
			return
		}
		name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, prefix), "/")
		index, exists := entries[name]
		isRoot := name == ""
		if !isRoot && !exists {
			http.NotFound(w, r)
			return
		}
		e := pullEntry{Directory: true}
		if !isRoot {
			e = o.entries[index]
		}
		if authlog.IsEnabled() {
			bit := map[string]uint32{"OPTIONS": 1, "PROPFIND": 2, "HEAD": 4, "GET": 8}[r.Method]
			if bit != 0 && observedMethods.Or(bit)&bit == 0 {
				traceClipboard("mac-dav-request", o.id, map[string]any{"method": r.Method, "root": isRoot, "directory": e.Directory, "range": r.Header.Get("Range") != ""})
			}
		}
		w.Header().Set("DAV", "1")
		w.Header().Set("Allow", "OPTIONS, PROPFIND, HEAD, GET")
		switch r.Method {
		case "OPTIONS":
			w.WriteHeader(http.StatusOK)
		case "PROPFIND":
			depth := r.Header.Get("Depth")
			if depth != "0" && depth != "1" {
				http.Error(w, "僅支援有限深度", http.StatusForbidden)
				return
			}
			result := davResponses{NS: "DAV:"}
			appendEntry := func(n string, v pullEntry) {
				u := url.URL{Path: prefix + n}
				if v.Directory && !strings.HasSuffix(u.Path, "/") {
					u.Path += "/"
				}
				p := davProperties{Name: path.Base(n), Size: v.Size, Modified: created.Format(http.TimeFormat), ETag: strconv.Quote(o.id + "-" + n)}
				if v.Directory {
					p.Type.Collection = &struct{}{}
				}
				result.Responses = append(result.Responses, davResponse{Href: u.EscapedPath(), Stat: davPropStat{Prop: p, Status: "HTTP/1.1 200 OK"}})
			}
			appendEntry(name, e)
			if depth == "1" && e.Directory {
				for _, v := range o.entries {
					parent := path.Dir(v.Name)
					if parent == "." {
						parent = ""
					}
					if parent == name {
						appendEntry(v.Name, v)
					}
				}
			}
			data, err := xml.Marshal(result)
			if err != nil {
				http.Error(w, "XML error", 500)
				return
			}
			w.Header().Set("Content-Type", "application/xml; charset=utf-8")
			w.WriteHeader(207)
			_, _ = w.Write(append([]byte(xml.Header), data...))
		case "GET", "HEAD":
			if e.Directory {
				http.Error(w, "directory", http.StatusMethodNotAllowed)
				return
			}
			// 預先提供型態，HEAD／中繼資料查詢不嗅探內容、不啟動傳輸。
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("ETag", strconv.Quote(o.id+"-"+name))
			http.ServeContent(&davProgressWriter{ResponseWriter: w}, r, path.Base(name), created, &davReader{offer: o, index: index})
		default:
			http.Error(w, "唯讀", http.StatusForbidden)
		}
	})
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second}
	go func() { _ = server.Serve(listener) }()
	pullMounts.Lock()
	pullMounts.roots[mount] = true
	pullMounts.Unlock()
	cleanup := func() {
		traceClipboard("mac-mount-closed", o.id, nil)
		pullMounts.Lock()
		delete(pullMounts.roots, mount)
		pullMounts.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = exec.CommandContext(ctx, "/sbin/umount", "-f", mount).Run()
		_ = server.Close()
		_ = os.Remove(mount)
	}
	ok := false
	defer func() {
		if !ok {
			cleanup()
		}
	}()
	ctx, cancel := context.WithTimeout(o.s.clipCtx, 20*time.Second)
	defer cancel()
	started := time.Now()
	traceClipboard("mac-mount-start", o.id, nil)
	output, err := exec.CommandContext(ctx, "/sbin/mount_webdav", "-S", "-o", "rdonly,nobrowse", "-v", "YourDesk", "http://"+listener.Addr().String()+prefix, mount).CombinedOutput()
	traceClipboard("mac-mount-finished", o.id, map[string]any{"success": err == nil, "elapsedMS": time.Since(started).Milliseconds(), "errorClass": authlog.ErrorClass(err)})
	if err != nil {
		return fmt.Errorf("無法建立 Finder 延遲貼上掛載：%w：%s", err, strings.TrimSpace(string(output)))
	}
	// 掛載成功不代表本 App 已獲准存取網路卷宗。先實際列舉目錄，讓
	// macOS 在需要時要求授權；只讀取清單，不預先下載遠端檔案內容。
	// 若未取得授權，pboard 可能拒收 file URL，但 writeObjects 仍回報成功。
	traceClipboard("mac-mount-access-start", o.id, nil)
	_, err = os.ReadDir(mount)
	traceClipboard("mac-mount-access-finished", o.id, map[string]any{"success": err == nil, "permissionDenied": errors.Is(err, os.ErrPermission)})
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			o.s.pull.permissionDeniedOffer = o.id
			return permissionError
		}
		return fmt.Errorf("無法讀取 Finder 貼上掛載：%w", err)
	}
	var roots []string
	for _, e := range o.entries {
		if path.Dir(e.Name) == "." {
			roots = append(roots, filepath.Join(mount, e.Name))
		}
	}
	if nativeRevision() != o.revision {
		traceClipboard("offer-rejected", o.id, map[string]any{"reason": "clipboard-changed-during-mount"})
		return fmt.Errorf("掛載期間本機已複製新內容，不覆寫剪貼簿")
	}
	if err = writeContent(content{Kind: "files", Paths: roots}); err != nil {
		traceClipboard("mac-pasteboard-write", o.id, map[string]any{"success": false, "roots": len(roots)})
		return err
	}
	traceClipboard("mac-pasteboard-write", o.id, map[string]any{"success": true, "roots": len(roots)})
	traceNativeFileClipboard(o.id, len(roots))
	o.s.pull.Lock()
	o.s.pull.cleanup = append(o.s.pull.cleanup, pullLease{offer: o, close: cleanup})
	o.s.pull.Unlock()
	ok = true
	return nil
}
