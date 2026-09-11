// Package clipboard 以獨立可靠通道同步文字、PNG 圖片、檔案與目錄。
package clipboard

import (
	"bytes"
	"errors"
	"image/png"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxTextBytes    = 1 * 1024 * 1024
	MaxImageBytes   = 32 * 1024 * 1024
	MaxFileBytes    = 2 * 1024 * 1024 * 1024
	MaxFiles        = 64
	MaxPixels       = 32 * 1024 * 1024
	TransferTimeout = 2 * time.Hour
)

type content struct {
	Kind  string
	Data  []byte
	Paths []string
}

func validImage(data []byte) error {
	if len(data) > MaxImageBytes {
		return errors.New("剪貼簿圖片過大")
	}
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 1 || config.Height < 1 || int64(config.Width)*int64(config.Height) > MaxPixels {
		return errors.New("剪貼簿圖片格式或尺寸無效")
	}
	return nil
}
func validText(data []byte) bool {
	return len(data) <= MaxTextBytes && utf8.Valid(data) && !strings.ContainsRune(string(data), 0)
}
