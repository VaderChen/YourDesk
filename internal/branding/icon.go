// Package branding 提供桌面視窗共用的品牌圖示。
package branding

import (
	"bytes"
	_ "embed"
	"image"
	"image/png"
)

//go:embed app-icon.png
var iconPNG []byte

func Icon() image.Image {
	icon, err := png.Decode(bytes.NewReader(iconPNG))
	if err != nil {
		return nil
	}
	return icon
}
