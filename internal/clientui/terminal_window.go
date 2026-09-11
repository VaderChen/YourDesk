//go:build cgo && (darwin || windows || linux)

package clientui

import (
	"context"
	"encoding/json"
	"errors"
	webview "github.com/webview/webview_go"
	"io"
	"net/url"
	"os"
	"sync"
)

// 每個終端機由獨立 Client 子程序持有原生視窗，不共用主視窗的事件迴圈。
func RunTerminalWindow(ctx context.Context) error {
	var in struct {
		URL   string `json:"url"`
		Title string `json:"title"`
	}
	decoder := json.NewDecoder(io.LimitReader(os.Stdin, 16384))
	if err := decoder.Decode(&in); err != nil {
		return err
	}
	u, err := url.Parse(in.URL)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Path != "/terminal.html" {
		return errors.New("終端機視窗位址無效")
	}
	window := webview.New(false)
	defer window.Destroy()
	if window.Window() == nil {
		return errors.New("無法建立終端機視窗")
	}
	var lifetime sync.Mutex
	alive := true
	dispatchClose := func() {
		lifetime.Lock()
		defer lifetime.Unlock()
		if alive {
			window.Dispatch(window.Terminate)
		}
	}
	defer func() { lifetime.Lock(); alive = false; lifetime.Unlock() }()
	window.SetTitle(in.Title)
	window.SetSize(520, 320, webview.HintMin)
	window.SetSize(980, 640, webview.HintNone)
	if err = window.Bind("yourdeskCloseTerminal", func() { dispatchClose() }); err != nil {
		return err
	}
	window.Navigate(in.URL)
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			dispatchClose()
		case <-done:
		}
	}()
	// 管理程序退出時私有管線也會關閉，避免遺留無法操作的視窗。
	go func() {
		_, _ = io.Copy(io.Discard, os.Stdin)
		select {
		case <-done:
			return
		default:
			dispatchClose()
		}
	}()
	window.Run()
	return nil
}
