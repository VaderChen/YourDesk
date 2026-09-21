//go:build (darwin || windows) && cgo

package main

import (
	"os"
	"strings"
	"testing"
)

// Smoke 使用實際嵌入、組裝後的 HTML，不能另行載入 JS 掩蓋組裝錯誤。
func TestTitlebarHTMLSmokeExport(t *testing.T) {
	if strings.Count(titlebarHTML, "<!-- VIRTUAL_KEYBOARD_SCRIPT -->") != 1 {
		t.Fatal("標題列鍵盤插入點必須唯一")
	}
	if !strings.HasSuffix(titlebarHTML, "<!-- VIRTUAL_KEYBOARD_SCRIPT --></body></html>\n") {
		t.Fatal("鍵盤腳本必須置於主腳本之外")
	}
	if output := os.Getenv("YOURDESK_TITLEBAR_SMOKE_HTML"); output != "" {
		if err := os.WriteFile(output, []byte(viewerTitlebarHTML()), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
