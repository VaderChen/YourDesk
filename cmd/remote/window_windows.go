//go:build windows && cgo

package main

/*
#cgo LDFLAGS: -luser32 -lgdi32
#include "window_windows.h"
#include <stdlib.h>
*/
import "C"

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/hajimehoshi/ebiten/v2"
	webview "github.com/webview/webview_go"
)

//go:embed web/titlebar_windows.js
var windowsTitlebarJS string

var winChrome struct {
	nativeMenu    atomic.Bool
	initialized   atomic.Bool
	transitioning atomic.Bool
	once          sync.Once
	mu            sync.Mutex
	view          webview.WebView
	state         windowsTitlebarState
	ready         atomic.Bool
	overlay       atomic.Bool
	visible       atomic.Bool
	popup         atomic.Bool
	actions       chan int
}

type windowsTitlebarState struct {
	Title            string                   `json:"title"`
	Strings          map[string]string        `json:"strings"`
	Language         string                   `json:"language"`
	Mode             int                      `json:"mode"`
	Quality          int                      `json:"quality"`
	Display          int                      `json:"display"`
	Count            int                      `json:"count"`
	Pending          bool                     `json:"pending"`
	TX               float64                  `json:"tx"`
	RX               float64                  `json:"rx"`
	RenderFPS        bool                     `json:"renderFPS"`
	FPS              float64                  `json:"fps"`
	VideoCodec       string                   `json:"videoCodec"`
	SourceEncoding   string                   `json:"sourceEncoding"`
	Enhancement      enhancementDisplayStatus `json:"enhancement"`
	ReceiverDecoding string                   `json:"receiverDecoding"`
	Fullscreen       bool                     `json:"fullscreen"`
	Maximized        bool                     `json:"maximized"`
}
type windowsTitlebarMessage struct {
	NativeMenu string  `json:"nativeMenu"`
	Ready      bool    `json:"ready"`
	Action     int     `json:"action"`
	Popup      bool    `json:"popup"`
	Menu       bool    `json:"menu"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	Width      float64 `json:"width"`
	Height     float64 `json:"height"`
}

func windowsDispatch(f func(webview.WebView)) {
	winChrome.mu.Lock()
	defer winChrome.mu.Unlock()
	if w := winChrome.view; w != nil {
		w.Dispatch(func() { f(w) })
	}
}

// 僅由遊戲更新執行緒存取，讓引擎與 Win32 的裝飾狀態一致。
var windowsDecorationHidden bool

// 在 RunGame 建立 HWND 前關閉裝飾，第一張畫面就不會出現系統標題列。
func nativePrepareViewerWindow() {
	ebiten.SetWindowDecorated(false)
	windowsDecorationHidden = true
}

func nativeConfigureTitlebar() {
	if hidden := !winChrome.initialized.Load() || winChrome.ready.Load(); hidden != windowsDecorationHidden {
		ebiten.SetWindowDecorated(!hidden)
		windowsDecorationHidden = hidden
	}

	winChrome.once.Do(func() {
		ebiten.SetRunnableOnUnfocused(true)
		winChrome.actions = make(chan int, 32)
		go runWindowsTitlebar()
	})
}
func runWindowsTitlebar() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer winChrome.initialized.Store(true)
	parent := C.yd_win_find_viewer()
	for parent == nil {
		time.Sleep(100 * time.Millisecond)
		parent = C.yd_win_find_viewer()
	}
	host := C.yd_win_create(parent)
	if host == nil {
		slog.Warn("Windows 遠端顯示 標題列建立失敗，保留備援工具列")
		return
	}
	defer C.yd_win_destroy()
	w := webview.NewWindow(false, host)
	if w.Window() == nil {
		w.Destroy()
		slog.Warn("Windows 遠端顯示 WebView2 無法啟動，保留備援工具列")
		return
	}
	defer w.Destroy()
	if err := w.Bind("ydTitlebar", func(message windowsTitlebarMessage) {
		if message.NativeMenu != "" {
			if !winChrome.nativeMenu.CompareAndSwap(false, true) {
				return
			}
			// 離開 WebView 的訊息回呼後才啟動系統選單事件迴圈。
			windowsDispatch(func(w webview.WebView) {
				defer winChrome.nativeMenu.Store(false)
				showWindowsNativeMenu(message)
			})
			return
		}
		if message.Ready {
			C.yd_win_activate()
			winChrome.ready.Store(true)
			winChrome.initialized.Store(true)
			return
		}
		if message.Popup {
			C.yd_win_popup(C.double(message.X), C.double(message.Y), C.double(message.Width), C.double(message.Height))
			wasOpen := winChrome.popup.Swap(message.Menu)
			if wasOpen && !message.Menu {
				C.yd_win_command(0)
			}
			return
		}
		switch message.Action {
		case 7, 8, 9:
			C.yd_win_command(C.int(message.Action))
		case 1, 2, 3, 4, 5, 6, 10, 11, 12, 13:
			C.yd_win_command(0)
			select {
			case winChrome.actions <- message.Action:
			default:
			}
		default:
			if message.Action >= 100 {
				C.yd_win_command(0)
				select {
				case winChrome.actions <- message.Action:
				default:
				}
			}
		}
	}); err != nil {
		slog.Warn("Windows 遠端顯示 工具列通道建立失敗", "error", err)
		return
	}
	page := strings.Replace(viewerTitlebarHTML(), "<script>", "<script>"+windowsTitlebarJS+"</script><script>", 1)
	windowsTitlebarStarted = time.Now()
	w.SetHtml(page)
	winChrome.mu.Lock()
	winChrome.view = w
	winChrome.mu.Unlock()
	done := make(chan struct{})
	defer close(done)
	// 合併更新，最多每 50ms 排一個工作；避免縮放期間 UI 佇列累積。
	var queued atomic.Bool
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if queued.CompareAndSwap(false, true) {
					windowsDispatch(func(w webview.WebView) { defer queued.Store(false); windowsTitlebarTick(w) })
				}
			}
		}
	}()
	w.Run()
	winChrome.mu.Lock()
	winChrome.view = nil
	winChrome.mu.Unlock()
	winChrome.ready.Store(false)
	winChrome.popup.Store(false)
}

var windowsTitlebarStarted time.Time
var lastWindowsTitlebarState string // 僅由 WebView UI 執行緒存取。
func windowsTitlebarTick(w webview.WebView) {
	flags := int(C.yd_win_tick())
	if flags < 0 {
		w.Terminate()
		return
	}
	if !winChrome.ready.Load() {
		if time.Since(windowsTitlebarStarted) > 15*time.Second {
			slog.Warn("Windows 遠端顯示 工具列載入逾時，保留備援工具列")
			w.Terminate()
		}
		return
	}
	winChrome.overlay.Store(flags&1 != 0)
	winChrome.visible.Store(flags&2 != 0)
	if flags&8 != 0 {
		w.Eval("window.closeWindowsPopup()")
		winChrome.popup.Store(false)
		C.yd_win_popup(0, 0, 0, 0)
	}
	var title [4096]C.char
	C.yd_win_title(&title[0], C.int(len(title)))
	winChrome.mu.Lock()
	winChrome.state.Title = C.GoString(&title[0])
	winChrome.state.Fullscreen = flags&1 != 0
	winChrome.state.Maximized = flags&4 != 0
	data, err := json.Marshal(winChrome.state)
	winChrome.mu.Unlock()
	if err == nil && string(data) != lastWindowsTitlebarState {
		lastWindowsTitlebarState = string(data)
		w.Eval("window.setTitlebarState(" + string(data) + ")")
	}
}
func nativeCloseTitlebar()            { windowsDispatch(func(w webview.WebView) { w.Terminate() }) }
func nativeTitlebarControls() bool    { return !winChrome.initialized.Load() || winChrome.ready.Load() }
func nativeTitlebarOverlay() bool     { return winChrome.overlay.Load() }
func nativeTitlebarVisible() bool     { return winChrome.visible.Load() }
func nativeTitlebarPopupOpen() bool   { return winChrome.popup.Load() || winChrome.nativeMenu.Load() }
func nativeFullscreenRequested() bool { return false }
func nativeRestoresWindowFrame() bool { return winChrome.ready.Load() }
func nativeFullscreenTransitioning() bool {
	return !winChrome.initialized.Load() || winChrome.transitioning.Load()
}
func nativeToggleFullscreen() bool {
	if !winChrome.initialized.Load() {
		return true
	}
	if !winChrome.ready.Load() {
		return false
	}
	winChrome.transitioning.Store(true)
	windowsDispatch(func(w webview.WebView) {
		w.Eval("window.closeWindowsPopup()")
		C.yd_win_popup(0, 0, 0, 0)
		winChrome.popup.Store(false)
		C.yd_win_command(4)
		windowsTitlebarTick(w)
		winChrome.transitioning.Store(false)
	})
	return true
}
func nativeTitlebarAction() int {
	select {
	case action := <-winChrome.actions:
		return action
	default:
		return 0
	}
}
func nativeSetTitlebarMode(mode int) {
	winChrome.mu.Lock()
	winChrome.state.Mode = mode
	winChrome.mu.Unlock()
}
func nativeSetQuality(quality int) {
	winChrome.mu.Lock()
	winChrome.state.Quality = quality
	winChrome.mu.Unlock()
}
func nativeSetDisplays(index, count int, pending bool) {
	winChrome.mu.Lock()
	winChrome.state.Display = index
	winChrome.state.Count = count
	winChrome.state.Pending = pending
	winChrome.mu.Unlock()
}
func nativeSetTraffic(tx, rx, fps float64, render bool) {
	winChrome.mu.Lock()
	winChrome.state.TX = tx
	winChrome.state.RX = rx
	winChrome.state.FPS = fps
	winChrome.state.RenderFPS = render
	winChrome.mu.Unlock()
}
func nativeSetCodecStatus(codec, source, receiver string) {
	winChrome.mu.Lock()
	winChrome.state.VideoCodec = codec
	winChrome.state.SourceEncoding = source
	winChrome.state.ReceiverDecoding = receiver
	winChrome.mu.Unlock()
}
func nativeSetLanguage(dictionary, language string) {
	if language == "auto" {
		var locale [128]C.char
		C.yd_win_locale(&locale[0], C.int(len(locale)))
		language = C.GoString(&locale[0])
	}
	index := -1
	switch {
	case strings.HasPrefix(language, "en"):
		index = 0
		language = "en"
	case strings.HasPrefix(language, "ja"):
		index = 1
		language = "ja"
	case strings.HasPrefix(language, "ko"):
		index = 2
		language = "ko"
	default:
		language = "zh-Hant"
	}
	var translations map[string][]string
	_ = json.Unmarshal([]byte(dictionary), &translations)
	values := make(map[string]string)
	if index >= 0 {
		for source, choices := range translations {
			if index < len(choices) {
				values[source] = choices[index]
			}
		}
	}
	winChrome.mu.Lock()
	winChrome.state.Strings = values
	winChrome.state.Language = language
	winChrome.mu.Unlock()
}
func nativeShowCloseConfirmation() {
	windowsDispatch(func(w webview.WebView) { w.Eval("window.showCloseConfirmation()") })
}

// 系統選單直接浮在桌面上，不受 WebView 背景與裁切區限制。
func showWindowsNativeMenu(message windowsTitlebarMessage) {
	winChrome.mu.Lock()
	state := winChrome.state
	winChrome.mu.Unlock()
	text := func(source string) string {
		if translated := state.Strings[source]; translated != "" {
			return translated
		}
		return source
	}
	menu := C.yd_win_menu_create()
	if menu == nil {
		return
	}
	defer C.yd_win_menu_destroy(menu)
	add := func(label string, action int, checked bool) {
		value := C.CString(label)
		defer C.free(unsafe.Pointer(value))
		mark := 0
		if checked {
			mark = 1
		}
		C.yd_win_menu_add(menu, value, C.int(action), C.int(mark))
	}
	switch message.NativeMenu {
	case "quality":
		for i, label := range []string{"低流量", "標準", "高畫質"} {
			add(text(label), 10+i, state.Quality == i)
		}
	case "scale":
		add(text("原始解析度（1:1）"), 1, state.Mode == 1)
		add(text("自動符合視窗"), 2, state.Mode == 2)
		add(text("只縮小，不放大"), 3, state.Mode == 0)
		add("", 0, false)
		label := "進入全螢幕"
		if state.Fullscreen {
			label = "離開全螢幕"
		}
		add(text(label), 4, state.Fullscreen)
	case "display":
		if state.Pending {
			return
		}
		for i := 0; i < state.Count; i++ {
			add(fmt.Sprintf(text("螢幕 %d"), i+1), 100+i, state.Display == i)
		}
	default:
		return
	}
	if action := int(C.yd_win_menu_show(menu, C.double(message.X), C.double(message.Y))); action != 0 {
		select {
		case winChrome.actions <- action:
		default:
		}
	}
}

func nativeSetEnhancementStatus(status enhancementDisplayStatus) {
	winChrome.mu.Lock()
	winChrome.state.Enhancement = status
	winChrome.mu.Unlock()
}
