package desktop

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"

	"github.com/kbinani/screenshot"
)

type Capturer interface {
	Capture(display int) (image.Image, error)
	Bounds(display int) image.Rectangle
}

type ScreenshotCapturer struct{}

func (ScreenshotCapturer) Count() int { return screenshot.NumActiveDisplays() }

func (ScreenshotCapturer) Bounds(display int) image.Rectangle {
	if display < 0 || display >= screenshot.NumActiveDisplays() {
		return image.Rectangle{}
	}
	return screenshot.GetDisplayBounds(display)
}

func (ScreenshotCapturer) Capture(display int) (image.Image, error) {
	if display < 0 || display >= screenshot.NumActiveDisplays() {
		return nil, fmt.Errorf("display %d 不存在（共 %d 個）", display, screenshot.NumActiveDisplays())
	}
	return captureDisplay(display)
}

func JPEG(img image.Image, quality int) ([]byte, error) {
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
