//go:build !darwin || !cgo

package frameinterp

import (
	"fmt"
	"image"
)

func load() error                                   { return fmt.Errorf("RIFE 目前僅支援 Apple Silicon Mac") }
func predict(a, b *image.RGBA) (*image.RGBA, error) { return nil, load() }

func AppleSupported() bool { return false }
func applePredict(a, b *image.RGBA) (*image.RGBA, error) {
	return nil, fmt.Errorf("Apple 補幀需要支援的 Mac 與 macOS 26 以上")
}

func AppleWorkingSize(w, h int) (int, int) { return w, h }
