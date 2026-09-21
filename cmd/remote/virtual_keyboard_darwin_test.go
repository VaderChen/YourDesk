//go:build darwin && cgo

package main

import (
	"testing"
	"time"
)

func TestVirtualKeyboardHitDoesNotWaitForAppKit(t *testing.T) {
	done := make(chan bool, 1)
	go func() { done <- nativeVirtualKeyboardPointerOver() }()
	select {
	case over := <-done:
		if over {
			t.Fatal("未開啟鍵盤不應攔截游標")
		}
	case <-time.After(time.Second):
		t.Fatal("鍵盤命中檢查不可同步等待 AppKit 主執行緒")
	}
}
