//go:build !windows

package desktop

import (
	"github.com/kbinani/screenshot"
	"image"
)

func captureDisplay(display int) (image.Image, error) {
	return screenshot.CaptureDisplay(display)
}
