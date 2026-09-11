// Package authlog 僅記錄握手階段與布林檢查結果，不接收密碼、金鑰、簽章或 SDP。
package authlog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Enabled 僅在診斷版建置時設為 1，正式版預設不寫入。
var Enabled string
var mu sync.Mutex
var file *os.File
var written int

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

func Start(version, mode string) {
	if Enabled != "1" {
		return
	}
	dir := os.Getenv("YOURDESK_AUTH_LOG_DIR")
	if dir == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			fmt.Fprintln(os.Stderr, "無法取得診斷紀錄路徑：", err)
			return
		}
		dir = filepath.Join(cache, "YourDesk", "Logs")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		fmt.Fprintln(os.Stderr, "無法建立診斷紀錄資料夾：", err)
		return
	}
	name := time.Now().Format("20060102-150405") + "-" + mode + "-" + formatPID() + ".jsonl"
	mu.Lock()
	var err error
	file, err = os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	mu.Unlock()
	if err != nil {
		fmt.Fprintln(os.Stderr, "無法開啟診斷紀錄檔：", err)
		return
	}
	Event("startup", map[string]any{"version": version, "mode": mode, "pid": os.Getpid(), "parentPid": os.Getppid()})
}
func formatPID() string { b, _ := json.Marshal(os.Getpid()); return string(b) }
func Event(event string, fields map[string]any) {
	if Enabled != "1" {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if file == nil || written >= 5<<20 {
		return
	}
	record := map[string]any{"time": time.Now().UTC().Format(time.RFC3339Nano), "event": event, "fields": fields}
	b, err := json.Marshal(record)
	if err != nil {
		return
	}
	b = append(b, '\n')
	n, _ := file.Write(b)
	written += n
	file.Sync()
}
