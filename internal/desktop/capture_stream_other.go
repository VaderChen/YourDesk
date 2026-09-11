//go:build !windows && (!darwin || !cgo)

package desktop

type compatibleCapturer struct{ ScreenshotCapturer }

func (compatibleCapturer) Close()   {}
func NewLiveCapturer() LiveCapturer { return compatibleCapturer{} }
