// Package authlog 記錄診斷版的握手階段、傳輸計數與停滯堆疊，不接收密碼、金鑰、簽章或 SDP。
package authlog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Enabled 保留舊建置參數相容性；是否寫入由網路 Debug 開關決定。
var Enabled string
var mu sync.Mutex
var file *os.File
var written int
var stackCount int

// ErrorClass 僅分類錯誤，不把服務端原始內容或連線資料寫入 LOG。
func ErrorClass(err error) string {
	if err == nil {
		return "none"
	}
	v := strings.ToLower(err.Error())
	for _, marker := range []string{"replaced", "duplicate", "already", "timeout", "deadline", "closed", "eof", "room", "role", "binding", "hmac"} {
		if strings.Contains(v, marker) {
			return marker
		}
	}
	return "other"
}

var processVersion, processMode string

func logDir() string {
	if d := os.Getenv("YOURDESK_AUTH_LOG_DIR"); d != "" {
		return d
	}
	cache, _ := os.UserCacheDir()
	return filepath.Join(cache, "YourDesk", "Logs")
}

// 所有同帳號程序共用開關；每次寫入前核對，關閉後不再寫入。
func IsEnabled() bool {
	_, err := os.Stat(filepath.Join(logDir(), "network-debug.enabled"))
	return err == nil
}
func SetDebug(enabled bool) error {
	if enabled {
		if err := os.MkdirAll(logDir(), 0700); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(logDir(), "network-debug.enabled"), []byte("1"), 0600)
	}
	err := os.Remove(filepath.Join(logDir(), "network-debug.enabled"))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
func Start(version, mode string) {
	mu.Lock()
	processVersion = version
	processMode = mode
	mu.Unlock()
	Event("startup", map[string]any{"version": version, "mode": mode, "pid": os.Getpid(), "parentPid": os.Getppid()})
}

// 呼叫端持有 mu；預設不建立任何日誌。
func openLog() {
	if file != nil || processMode == "" {
		return
	}
	dir := logDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return
	}
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".jsonl") {
			if info, err := entry.Info(); err == nil && time.Since(info.ModTime()) > 7*24*time.Hour {
				_ = os.Remove(filepath.Join(dir, entry.Name()))
			}
		}
	}
	name := time.Now().Format("20060102-150405") + "-" + processMode + "-" + formatPID() + ".jsonl"
	var err error
	file, err = os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		fmt.Fprintln(os.Stderr, "無法開啟診斷紀錄檔：", err)
		return
	}
	written = 0
	stackCount = 0
	b, _ := json.Marshal(map[string]any{"time": time.Now().UTC().Format(time.RFC3339Nano), "event": "startup", "fields": map[string]any{"version": processVersion, "mode": processMode, "pid": os.Getpid(), "parentPid": os.Getppid()}})
	n, _ := file.Write(append(b, '\n'))
	written += n
}
func formatPID() string { b, _ := json.Marshal(os.Getpid()); return string(b) }
func Event(event string, fields map[string]any) {
	if !IsEnabled() {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if !IsEnabled() {
		return
	}
	openLog()
	if file == nil || written >= 5<<20 {
		return
	}
	record := map[string]any{"time": time.Now().UTC().Format(time.RFC3339Nano), "event": event, "fields": fields}
	b, err := json.Marshal(record)
	if err != nil {
		return
	}
	if written+len(b)+1 > 5<<20 {
		return
	}
	b = append(b, '\n')
	n, _ := file.Write(b)
	written += n
	file.Sync()
}

// Stacks 每程序至多兩次；不擷取記憶體內容或輸入資料。
func Stacks() {
	if !IsEnabled() {
		return
	}
	mu.Lock()
	if file == nil || stackCount >= 2 {
		mu.Unlock()
		return
	}
	stackCount++
	mu.Unlock()
	buf := make([]byte, 512<<10)
	n := runtime.Stack(buf, true)
	Event("goroutines", map[string]any{"stack": string(buf[:n]), "truncated": n == len(buf)})
}
