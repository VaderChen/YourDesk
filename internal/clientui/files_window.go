//go:build cgo && (darwin || windows || linux)

package clientui

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	webview "github.com/webview/webview_go"
)

// 每個傳檔視窗獨立子程序；JS 綁定只改變背景工作狀態，不等待網路或磁碟。
func RunFilesWindow(parent context.Context) error {
	pipe := bufio.NewScanner(os.Stdin)
	pipe.Buffer(make([]byte, 4096), 16385)
	if !pipe.Scan() {
		if err := pipe.Err(); err != nil {
			return err
		}
		return io.EOF
	}
	in, err := decodeFilesWindowMessage(pipe.Bytes(), true)
	if err != nil {
		return err
	}
	u, params, err := parseFilesAddress(in.URL)
	if err != nil {
		return err
	}
	ctx, stop := context.WithCancel(parent)
	defer stop()
	transport := &http.Transport{Proxy: nil, MaxConnsPerHost: 2}
	defer transport.CloseIdleConnections()
	client := &fileDownloadClient{endpoint: u.Scheme + "://" + u.Host + "/api/files", token: params.Get("token"), session: params.Get("session"), instance: params.Get("instance"),
		client: &http.Client{Transport: transport, Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("不允許下載連線重新導向") }}}
	window := webview.New(false)
	defer window.Destroy()
	if window.Window() == nil {
		return errors.New("無法建立檔案傳輸視窗")
	}
	window.SetTitle(in.Title)
	window.SetSize(700, 560, webview.HintMin)
	window.SetSize(1000, 740, webview.HintNone)
	var lifetime sync.Mutex
	alive := true
	dispatch := func(f func()) {
		lifetime.Lock()
		defer lifetime.Unlock()
		if alive {
			window.Dispatch(func() {
				lifetime.Lock()
				active := alive
				lifetime.Unlock()
				// f runs on the UI thread, as does teardown. A state change
				// may queue its own progress, so do not hold this mutex.
				if active {
					f()
				}
			})
		}
	}
	emit := func(name string, value any) {
		raw, _ := json.Marshal(value)
		event, _ := json.Marshal(name)
		window.Eval("window.dispatchEvent(new CustomEvent(" + string(event) + ",{detail:" + string(raw) + "}));")
	}
	closeGate := &filesCloseGate{request: func() { emit("yourdesk-request-close", nil) }, terminate: func() { stop(); window.Terminate() }}
	removeClose, err := installFilesClose(window.Window(), closeGate.requestClose)
	if err != nil {
		return err
	}
	defer removeClose()
	removePicker, err := installFilesPicker(window.Window())
	if err != nil {
		return err
	}
	defer removePicker()
	var jobMu sync.Mutex
	var current *fileDownloadControl
	var workers sync.WaitGroup
	var lastProgress uint64 // UI thread only; reset for each preparation.
	defer func() {
		lifetime.Lock()
		alive = false
		lifetime.Unlock()
		stop()
		jobMu.Lock()
		job := current
		jobMu.Unlock()
		if job != nil {
			job.stop()
		}
		workers.Wait()
	}()
	directorySlot := make(chan struct{}, 1)
	if err = window.Bind("yourdeskStartDirectoryList", func(path string, offset int, id string) error {
		if len(path) > 4096 || id == "" || len(id) > 64 {
			return errors.New("目錄參數無效")
		}
		select {
		case directorySlot <- struct{}{}:
		default:
			return errors.New("正在讀取本機目錄")
		}
		workers.Add(1)
		go func() {
			defer workers.Done()
			defer func() { <-directorySlot }()
			out, err := listLocalDirectories(ctx, path, offset)
			dispatch(func() {
				result := map[string]any{"id": id, "result": out}
				if err != nil {
					result["error"] = err.Error()
				}
				emit("yourdesk-directory-result", result)
			})
		}()
		return nil
	}); err != nil {
		return err
	}
	if err = window.Bind("yourdeskStartFile", func(path, directory, id string) error {
		if len(path) > 2048 || path == "" || len(id) > 64 || id == "" {
			return errors.New("下載參數無效")
		}
		jobMu.Lock()
		if current != nil {
			jobMu.Unlock()
			return errors.New("請等待目前下載完成或取消")
		}
		jobMu.Unlock()
		if directory == "" {
			return errors.New("請先選擇下載目錄")
		}
		destination, err := localDirectoryPath(directory)
		if err != nil {
			return err
		}
		var job *fileDownloadControl
		job = newFileDownloadControl(ctx, func(progress fileProgress) {
			dispatch(func() {
				jobMu.Lock()
				active := current == job
				jobMu.Unlock()
				if active && progress.Sequence > lastProgress {
					lastProgress = progress.Sequence
					emit("yourdesk-file-progress", progress)
				}
			})
		})
		jobMu.Lock()
		current = job
		jobMu.Unlock()
		lastProgress = 0
		workers.Add(1)
		go func() {
			defer workers.Done()
			out, err := client.prepareControlledAt(job, path, destination)
			dispatch(func() {
				if err == nil {
					err = job.ctx.Err()
				}
				jobMu.Lock()
				active := current == job
				if active {
					current = nil
				}
				jobMu.Unlock()
				job.cancel()
				if !active {
					return
				}
				result := map[string]any{"id": id, "name": out.Name, "size": out.Size, "path": out.Path}
				if err != nil {
					result["error"] = err.Error()
				}
				emit("yourdesk-file-result", result)
			})
		}()
		return nil
	}); err != nil {
		return err
	}
	activeJob := func() *fileDownloadControl { jobMu.Lock(); defer jobMu.Unlock(); return current }
	// Recover a session update that arrived before the page installed listeners.
	if err = window.Bind("yourdeskFilesSession", func() map[string]string {
		client.mu.RLock()
		defer client.mu.RUnlock()
		return map[string]string{"instance": client.instance}
	}); err != nil {
		return err
	}
	if err = window.Bind("yourdeskPauseFile", func() (map[string]any, error) {
		job := activeJob()
		if job == nil {
			return nil, errors.New("目前沒有可暫停的下載")
		}
		progress, err := job.pause()
		if err != nil {
			return nil, err
		}
		return map[string]any{"paused": true, "received": progress.Received, "total": progress.Total}, nil
	}); err != nil {
		return err
	}
	if err = window.Bind("yourdeskResumeFile", func() (map[string]bool, error) {
		job := activeJob()
		if job == nil {
			return nil, errors.New("目前沒有可繼續的下載")
		}
		if err := job.resume(); err != nil {
			return nil, err
		}
		return map[string]bool{"resumed": true}, nil
	}); err != nil {
		return err
	}
	if err = window.Bind("yourdeskCancelFile", func() {
		if job := activeJob(); job != nil {
			job.stop()
		}
	}); err != nil {
		return err
	}
	// 頁面的關閉攔截就緒後喚起一次；重連不重新載入頁面。
	var readyOnce sync.Once
	if err = window.Bind("yourdeskFilesReady", func() {
		closeGate.markReady()
		readyOnce.Do(func() {
			dispatch(func() {
				if ctx.Err() == nil {
					showUpdateWindow(window.Window())
				}
			})
		})
	}); err != nil {
		return err
	}
	if err = window.Bind("yourdeskCloseFiles", closeGate.forceClose); err != nil {
		return err
	}
	// Interruption keeps this Promise pending until explicit resume/cancel.
	window.Init(`window.yourdeskListDirectories = (path,offset=0) => new Promise((resolve,reject) => {
	 const id=crypto.randomUUID();
	 const done=event=>{if(event.detail.id!==id)return;window.removeEventListener('yourdesk-directory-result',done);event.detail.error?reject(new Error(event.detail.error)):resolve(event.detail.result)};
	 window.addEventListener('yourdesk-directory-result',done);
	 window.yourdeskStartDirectoryList(path,offset,id).catch(error=>{window.removeEventListener('yourdesk-directory-result',done);reject(new Error(String(error)))});
	});
	window.yourdeskPrepareFile = (path,directory) => new Promise((resolve,reject) => {
	 const id=crypto.randomUUID();
	 const done=event=>{if(event.detail.id!==id)return;window.removeEventListener('yourdesk-file-result',done);event.detail.error?reject(new Error(event.detail.error)):resolve(event.detail)};
	 window.addEventListener('yourdesk-file-result',done);
	 window.yourdeskStartFile(path,directory,id).catch(error=>{window.removeEventListener('yourdesk-file-result',done);reject(new Error(String(error)))});
	});`)
	window.Navigate(in.URL)
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			dispatch(closeGate.forceClose)
		case <-done:
		}
	}()
	pipeErrors := make(chan error, 1)
	go func() {
		defer func() { stop(); dispatch(closeGate.forceClose) }()
		for pipe.Scan() {
			message, err := decodeFilesWindowMessage(pipe.Bytes(), false)
			if err != nil {
				pipeErrors <- err
				return
			}
			if message.Event == "files-focus" {
				dispatch(func() { showUpdateWindow(window.Window()) })
				continue
			}
			address, next, err := parseFilesAddress(message.URL)
			if err != nil || address.Scheme+"://"+address.Host+"/api/files" != client.endpoint || next.Get("token") != client.token || next.Get("session") != client.session {
				pipeErrors <- errors.New("不允許更換檔案視窗的來源或站台")
				return
			}
			dispatch(func() {
				client.mu.RLock()
				previous := client.instance
				client.mu.RUnlock()
				if previous != next.Get("instance") {
					if job := activeJob(); job != nil {
						job.interrupt(nil)
					}
					instance, err := client.rebind(message.URL)
					if err != nil {
						closeGate.forceClose()
						return
					}
					emit("yourdesk-files-session", map[string]string{"instance": instance})
				}
				if message.Title != "" {
					window.SetTitle(message.Title)
				}
				showUpdateWindow(window.Window())
			})
		}
		if err := pipe.Err(); err != nil {
			pipeErrors <- err
		}
	}()
	window.Run()
	select {
	case err := <-pipeErrors:
		return err
	default:
		return nil
	}
}
