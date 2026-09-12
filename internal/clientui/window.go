//go:build cgo && (darwin || windows || linux)

package clientui

import (
	"context"
	"errors"
	webview "github.com/webview/webview_go"
	"runtime"
	"time"
	"yourdesk/internal/childprocess"
)

func windowAvailable() error { return nil }

// WebView 的 init 將主 goroutine 固定在 OS 主執行緒，以符合原生 UI 要求。
func runWindow(ctx context.Context, address string, connected <-chan struct{}, updates <-chan struct{}, transfers <-chan struct{}, viewerClosed <-chan struct{}, app *server) error {
	window := webview.New(false)
	defer window.Destroy()
	if window.Window() == nil {
		return errors.New("無法建立桌面視窗，請確認系統 WebView 元件已安裝")
	}
	window.SetTitle("YourDesk · 裝置與站台")
	window.SetSize(820, 640, webview.HintMin)
	window.SetSize(1040, 720, webview.HintNone)
	removeTray, err := installTray(window.Window(), window.Terminate, func() { go app.showMCPWindows() }, func() { go app.stopIncomingFromTray() })
	if err != nil {
		return err
	}
	defer removeTray()
	// 固定的外部連結共用系統瀏覽器入口，不依賴 WebView 的新視窗支援。
	if err := window.Bind("yourdeskOpenExternal", func(url string) error {
		switch url {
		case "https://github.com/VaderChen/YourDesk", "https://buymeacoffee.com/vaderchen":
		default:
			return errors.New("不支援的外部連結")
		}
		openCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		switch runtime.GOOS {
		case "darwin":
			return childprocess.CommandContext(openCtx, "/usr/bin/open", url).Run()
		case "windows":
			return childprocess.CommandContext(openCtx, "rundll32.exe", "url.dll,FileProtocolHandler", url).Run()
		default:
			return childprocess.CommandContext(openCtx, "xdg-open", url).Run()
		}
	}); err != nil {
		return err
	}
	if err := window.Bind("yourdeskWindowAction", func(action string) {
		window.Dispatch(func() {
			switch action {
			case "close":
				closeInterfaceWindow(window.Window())
			case "quit":
				window.Terminate()
			}
		})
	}); err != nil {
		return err
	}
	window.Init(`document.addEventListener('keydown',e=>{
 if(e.repeat||e.isComposing||e.shiftKey)return;
 const mac=/Mac/.test(navigator.platform), command=mac?e.metaKey:e.ctrlKey;
 let action='';if(command&&!e.altKey&&e.key.toLowerCase()==='w')action='close';
 if(mac&&e.metaKey&&!e.ctrlKey&&!e.altKey&&e.key.toLowerCase()==='q')action='quit';
 if(!mac&&e.altKey&&!e.ctrlKey&&!e.metaKey&&e.key==='F4')action='close';
 if(action){e.preventDefault();e.stopImmediatePropagation();window.yourdeskWindowAction(action);}
 },true);`)
	window.Navigate(address)
	finished := make(chan struct{})
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		lastMCP := -1
		lastVisible := false
		lastIncoming := false
		lastIncomingCheck := time.Time{}
		for {
			select {
			case <-ticker.C:
				if time.Since(lastIncomingCheck) >= time.Second {
					lastIncomingCheck = time.Now()
					incoming := app.incomingConnected()
					app.mu.Lock()
					app.incomingActive = incoming
					app.mu.Unlock()
					if incoming != lastIncoming {
						lastIncoming = incoming
						window.Dispatch(func() { updateIncomingTray(window.Window(), incoming) })
					}
				}
				count, visible := app.mcpSessionCount()
				if count != lastMCP || visible != lastVisible {
					notify := count > lastMCP && count > 0
					lastMCP = count
					lastVisible = visible
					window.Dispatch(func() { updateMCPTray(window.Window(), count, notify, visible) })
				}
			case <-ctx.Done():
				// Windows 的 PostQuitMessage 必須在 UI 執行緒送出。
				window.Dispatch(window.Terminate)
				return
			case <-finished:
				return
			case <-transfers:
				window.Dispatch(func() {
					showTransferWindow(window.Window())
					window.Eval("window.dispatchEvent(new Event('yourdesk-transfer'))")
				})
			case <-updates:
				window.Dispatch(func() {
					showUpdateWindow(window.Window())
					window.Eval("window.dispatchEvent(new Event('yourdesk-update'))")
				})
			case <-viewerClosed:
				// 遠端顯示 退出優先於尚未顯示的連線通知，避免主畫面再次被隱藏。
				for len(connected) > 0 {
					<-connected
				}
				window.Dispatch(func() {
					select {
					case <-ctx.Done():
						return
					case <-finished:
						return
					default:
					}
					showUpdateWindow(window.Window())
				})
			case <-connected:
				window.Dispatch(func() { showConnectionNotice(window.Window()) })
			}
		}
	}()
	window.Run()
	close(finished)
	// 確保 Terminate 不會與原生視窗的 Destroy 同時執行。
	<-watcherDone
	return nil
}
