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

func viewerTitlebarHTML() string {
	return strings.ReplaceAll(strings.ReplaceAll(titlebarHTML, "FONT_AWESOME_DATA", base64.StdEncoding.EncodeToString(titlebarFont)), "FONT_AWESOME_LICENSE", titlebarFontLicense)
}
