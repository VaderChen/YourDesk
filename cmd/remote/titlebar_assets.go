//go:build (darwin || windows) && cgo

package main

import (
	_ "embed"
	"encoding/base64"
	"strings"
)

//go:embed web/titlebar.html
var titlebarHTML string

//go:embed web/fontawesome/fa-solid-900.woff2
var titlebarFont []byte

//go:embed web/fontawesome/LICENSE.txt
var titlebarFontLicense string

//go:embed web/virtual_keyboard.js
var virtualKeyboardJS string

func viewerTitlebarHTML() string {
	// 同一份鍵盤用於 macOS WebView 面板及 Windows WebView2。
	html := strings.Replace(titlebarHTML, "<!-- VIRTUAL_KEYBOARD_SCRIPT -->", "<script>"+virtualKeyboardJS+"</script>", 1)
	return strings.ReplaceAll(strings.ReplaceAll(html, "FONT_AWESOME_DATA", base64.StdEncoding.EncodeToString(titlebarFont)), "FONT_AWESOME_LICENSE", titlebarFontLicense)
}
