//go:build windows && !cgo

package main

import (
	"errors"
	"flag"
	"io"
	"log/slog"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
	"yourdesk/internal/peertransport"
)

func main() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// 救援密碼僅放在原生視窗；不輸出到共用 PE 記錄或重導向 stdout。
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := run(); err != nil {
		messageBox("YourDesk WinPE (Experimental)", err.Error())
	}
}
func run() error {
	if !peertransport.RestrictedBuild {
		return errors.New("Build with scripts/build-winpe.sh (CGO_ENABLED=0, -tags winpe).")
	}
	flags := flag.NewFlagSet("yourdesk-winpe", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	signal := flags.String("signal", "wss://desktop.mars-cloud.com:8080/ws", "Pairing server WSS URL")
	fps := flags.Int("fps", 8, "Capture FPS: 1-20")
	quality := flags.Int("quality", 60, "JPEG quality: 20-90")
	transport := flags.String("transport", "", "Transport (Tailcat unavailable in WinPE experimental build)")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("Unexpected command-line arguments")
	}
	if !peertransport.Supports(peertransport.Mode(*transport)) {
		return errors.New("Transport disabled / 傳輸模式停用: " + peertransport.UnavailableReason(peertransport.Mode(*transport)))
	}
	// 同時以核心版本與必要 API 探測，避免僅用 PE 映像名稱判斷相容性。
	if windows.RtlGetVersion().MajorVersion < 10 {
		return errors.New("Requires Windows 10/11 based WinPE x64 (experimental).")
	}
	for dll, procedures := range map[string][]string{
		"user32.dll":  {"SendInput", "CreateWindowExW", "GetDC", "EnumDisplayMonitors"},
		"gdi32.dll":   {"BitBlt", "CreateDIBSection"},
		"ws2_32.dll":  {"WSAStartup"},
		"crypt32.dll": {"CertOpenSystemStoreW"},
	} {
		lib := windows.NewLazySystemDLL(dll)
		for _, name := range procedures {
			if err := lib.NewProc(name).Find(); err != nil {
				return errors.New("Required API unavailable: " + dll + " / " + name)
			}
		}
	}
	host, err := newRescueHost(*signal, *fps, *quality)
	if err != nil {
		return err
	}
	defer host.stop()
	// 防止同一登入工作階段重複啟動救援 Host；不使用會被映像複製的 MachineGuid。
	name, _ := windows.UTF16PtrFromString(`Local\YourDesk.WinPE.Experimental`)
	mutex, err := windows.CreateMutex(nil, false, name)
	if mutex != 0 {
		defer windows.CloseHandle(mutex)
	}
	if err != nil {
		if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			return errors.New("YourDesk WinPE is already running / 已在執行")
		}
		return err
	}
	return showWindow(host)
}
func messageBox(title, message string) {
	caption, _ := windows.UTF16PtrFromString(title)
	body, _ := windows.UTF16PtrFromString(message)
	windows.NewLazySystemDLL("user32.dll").NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(body)), uintptr(unsafe.Pointer(caption)), 0x10)
	runtime.KeepAlive(caption)
	runtime.KeepAlive(body)
}
