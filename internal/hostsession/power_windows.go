//go:build windows

package hostsession

import (
	"log/slog"
	"runtime"
	"sync"

	"golang.org/x/sys/windows"
)

var setThreadExecutionState = windows.NewLazySystemDLL("kernel32.dll").NewProc("SetThreadExecutionState")

// 僅在已建立的桌面連線期間生效，不改系統電源設定或螢幕保護／鎖定規則。
func keepSessionAwake() func() {
	stop, done := make(chan struct{}), make(chan struct{})
	ready := make(chan struct{})
	go func() {
		// 電源需求屬於 OS 執行緒；使用專用執行緒並在結束時退役，
		// 不讓 Go 排程把清除操作移到另一條執行緒。
		runtime.LockOSThread()
		defer close(done)
		const continuous = 0x80000000
		previous, _, _ := setThreadExecutionState.Call(continuous | 0x0001 | 0x0002)
		if previous == 0 {
			slog.Warn("Windows 未接受遠端連線的防休眠要求")
			close(ready)
			return
		}
		close(ready)
		<-stop
		if ok, _, _ := setThreadExecutionState.Call(continuous); ok == 0 {
			slog.Warn("Windows 防休眠要求清除失敗，結束專用執行緒")
		}
		// 不 UnlockOSThread：goroutine 結束會終止這條 OS 執行緒，
		// 即使清除失敗也不把電源需求遺留給共用執行緒池。
	}()
	<-ready
	var once sync.Once
	return func() {
		once.Do(func() { close(stop) })
		<-done
	}
}
