//go:build windows && !cgo

package main

import (
	"errors"
	"runtime"
	"time"
	"unsafe"

	"github.com/lxn/win"
	"golang.org/x/sys/windows"
	"yourdesk/internal/peertransport"
)

func wide(s string) *uint16 { p, _ := windows.UTF16PtrFromString(s); return p }
func setText(hwnd win.HWND, text string) {
	p := wide(text)
	win.SendMessage(hwnd, win.WM_SETTEXT, 0, uintptr(unsafe.Pointer(p)))
	runtime.KeepAlive(p)
}
func showWindow(host *rescueHost) error {
	if err := windows.NewLazySystemDLL("comctl32.dll").NewProc("InitCommonControlsEx").Find(); err != nil {
		return errors.New("WinPE common controls are unavailable")
	}
	icc := win.INITCOMMONCONTROLSEX{DwICC: win.ICC_WIN95_CLASSES}
	icc.DwSize = uint32(unsafe.Sizeof(icc))
	if !win.InitCommonControlsEx(&icc) {
		return errors.New("Cannot initialize Win32 controls")
	}
	var window, status, start, stop, password, show win.HWND
	var closing time.Time
	shown := false
	lastStatus := ""
	refresh := func() {
		value, listening, busy := host.status()
		if value != lastStatus {
			setText(status, value)
			lastStatus = value
		}
		win.EnableWindow(start, !busy && closing.IsZero())
		win.EnableWindow(stop, listening && closing.IsZero())
		if !closing.IsZero() && (!busy || time.Since(closing) > 5*time.Second) {
			win.DestroyWindow(window)
		}
	}
	proc := windows.NewCallback(func(hwnd win.HWND, msg uint32, wParam, lParam uintptr) uintptr {
		switch msg {
		case win.WM_COMMAND:
			switch uint16(wParam) {
			case 101:
				if closing.IsZero() {
					host.start()
					refresh()
				}
			case 102:
				host.stop()
				refresh()
			case 103:
				shown = !shown
				mask := uintptr('*')
				label := "Show / 顯示"
				if shown {
					mask = 0
					label = "Hide / 隱藏"
				}
				win.SendMessage(password, win.EM_SETPASSWORDCHAR, mask, 0)
				win.InvalidateRect(password, nil, true)
				setText(show, label)
			}
			return 0
		case win.WM_TIMER:
			refresh()
			return 0
		case win.WM_CLOSE:
			if closing.IsZero() {
				closing = time.Now()
				host.stop()
				refresh()
			}
			return 0
		case win.WM_DESTROY:
			win.KillTimer(hwnd, 1)
			host.stop()
			win.PostQuitMessage(0)
			return 0
		}
		return win.DefWindowProc(hwnd, msg, wParam, lParam)
	})
	instance := win.GetModuleHandle(nil)
	className := wide("YourDeskWinPERescue")
	wc := win.WNDCLASSEX{LpfnWndProc: proc, HInstance: instance, LpszClassName: className, HbrBackground: win.HBRUSH(win.COLOR_BTNFACE + 1)}
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	if win.RegisterClassEx(&wc) == 0 {
		return errors.New("Cannot register rescue window")
	}
	window = win.CreateWindowEx(0, className, wide("YourDesk WinPE — Experimental / 實驗性"), win.WS_OVERLAPPED|win.WS_CAPTION|win.WS_SYSMENU|win.WS_MINIMIZEBOX, win.CW_USEDEFAULT, win.CW_USEDEFAULT, 650, 390, 0, 0, instance, nil)
	if window == 0 {
		return errors.New("Cannot create rescue window")
	}
	defer win.DestroyWindow(window)
	font := win.GetStockObject(win.DEFAULT_GUI_FONT)
	failed := false
	child := func(class, label string, style uint32, x, y, width, height int32, id uintptr) win.HWND {
		h := win.CreateWindowEx(0, wide(class), wide(label), win.WS_CHILD|win.WS_VISIBLE|style, x, y, width, height, window, win.HMENU(id), instance, nil)
		if h == 0 {
			failed = true
		} else {
			win.SendMessage(h, win.WM_SETFONT, uintptr(font), 1)
		}
		return h
	}
	heading := child("STATIC", "YourDesk WinPE  [EXPERIMENTAL / 實驗性]", win.SS_NOTIFY, 22, 18, 595, 28, 0)
	child("STATIC", "Temporary ID / 臨時 ID", 0, 22, 60, 180, 24, 0)
	child("EDIT", host.room, win.WS_BORDER|win.ES_READONLY|win.ES_AUTOHSCROLL|win.WS_TABSTOP, 204, 56, 408, 28, 0)
	child("STATIC", "Password / 連線密碼", 0, 22, 103, 180, 24, 0)
	password = child("EDIT", host.password, win.WS_BORDER|win.ES_READONLY|win.ES_PASSWORD|win.ES_AUTOHSCROLL|win.WS_TABSTOP, 204, 98, 285, 28, 0)
	show = child("BUTTON", "Show / 顯示", win.BS_PUSHBUTTON|win.WS_TABSTOP, 498, 98, 114, 28, 103)
	child("STATIC", "GDI / Software JPEG / Native P2P", 0, 22, 143, 590, 24, 0)
	toggle := child("BUTTON", "Tailcat  [Experimental / 實驗性]", win.BS_AUTOCHECKBOX, 22, 179, 400, 27, 0)
	win.EnableWindow(toggle, false)
	help := child("STATIC", "(?)", win.SS_NOTIFY|win.WS_TABSTOP, 435, 179, 45, 27, 0)
	status = child("EDIT", "", win.WS_BORDER|win.ES_READONLY|win.ES_AUTOHSCROLL, 22, 225, 590, 28, 0)
	start = child("BUTTON", "Start / 開始接收", win.BS_PUSHBUTTON|win.WS_TABSTOP, 22, 278, 205, 34, 101)
	stop = child("BUTTON", "Stop / 停止遠端連線", win.BS_PUSHBUTTON|win.WS_TABSTOP, 240, 278, 260, 34, 102)
	if failed {
		return errors.New("Cannot create rescue controls")
	}
	tooltip := win.CreateWindowEx(win.WS_EX_TOPMOST, wide("tooltips_class32"), nil, win.WS_POPUP|win.TTS_ALWAYSTIP|win.TTS_NOPREFIX, 0, 0, 0, 0, window, 0, instance, nil)
	if tooltip == 0 {
		return errors.New("Cannot create help tooltips")
	}
	win.SendMessage(tooltip, win.TTM_SETMAXTIPWIDTH, 0, 430)
	// TOOLINFO 文字由 Windows 持有，保留到視窗結束，避免 Go 回收字串。
	tips := make([]*uint16, 0, 3)
	defer func() { win.DestroyWindow(tooltip); runtime.KeepAlive(tips) }()
	addTip := func(target win.HWND, value string) {
		text := wide(value)
		tips = append(tips, text)
		info := win.TOOLINFO{UFlags: win.TTF_IDISHWND | win.TTF_SUBCLASS, Hwnd: window, UId: uintptr(target), LpszText: text}
		info.CbSize = uint32(unsafe.Sizeof(info))
		win.SendMessage(tooltip, win.TTM_ADDTOOL, 0, uintptr(unsafe.Pointer(&info)))
	}
	addTip(help, "Tailcat unavailable in this experimental WinPE build. / "+peertransport.UnavailableReason(peertransport.Tailcat))
	addTip(heading, "Experimental recovery Host. Temporary ID/password change on restart. Clipboard, file transfer, and remote shell are disabled. / 實驗性救援 Host；每次啟動更換 ID 與密碼，停用剪貼簿、檔案傳送與遠端 Shell。")
	addTip(stop, "Stop accepting connections and disconnect the current remote session. / 停止接收連線，並中斷目前遠端工作階段。")
	if win.SetTimer(window, 1, 250, 0) == 0 {
		return errors.New("Cannot create status timer")
	}
	host.start()
	refresh()
	win.ShowWindow(window, win.SW_SHOW)
	win.UpdateWindow(window)
	var msg win.MSG
	for {
		result := win.GetMessage(&msg, 0, 0, 0)
		if result == 0 {
			break
		}
		if result == -1 {
			return errors.New("Window message loop failed")
		}
		if !win.IsDialogMessage(window, &msg) {
			win.TranslateMessage(&msg)
			win.DispatchMessage(&msg)
		}
	}
	runtime.KeepAlive(tips)
	runtime.KeepAlive(className)
	return nil
}
