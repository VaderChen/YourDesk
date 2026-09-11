package remotedata

import (
	"errors"
	"os"
	"strings"
	"sync"
)

// 持續消耗輸出，只保留有限內容，避免管線塞住或回應過大。
type outputBuffer struct {
	mu        sync.Mutex
	data      []byte
	truncated bool
}

func (b *outputBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	remaining := 2048 - len(b.data)
	if n > remaining {
		b.truncated = true
		p = p[:remaining]
	}
	b.data = append(b.data, p...)
	return n, nil
}
func (b *outputBuffer) result(code int, timedOut bool, shell string) any {
	b.mu.Lock()
	defer b.mu.Unlock()
	return map[string]any{"output": strings.ToValidUTF8(string(b.data), "�"), "exitCode": code, "timedOut": timedOut, "truncated": b.truncated, "shell": shell}
}
func checkRegular(f *os.File, err error) (*os.File, error) {
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		f.Close()
		if err == nil {
			err = errors.New("只能讀取一般檔案")
		}
		return nil, err
	}
	return f, nil
}
