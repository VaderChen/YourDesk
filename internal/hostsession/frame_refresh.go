package hostsession

import (
	"errors"
	"image"
	"time"
	"yourdesk/internal/desktop"
)

const idleJPEGRefreshInterval = 5 * time.Second

// 當最後一個 patch 在網路遺失時，Viewer 沒有後續序號可發現缺口。
// JPEG 靜止桌面仍定期擷取一次，讓既有五秒完整影格機制確實有機會執行。
// 只由 Capture worker 存取；無新畫面時不每輪複製舊 buffer。
type idleFrameRefresh struct{ next time.Time }

func (r *idleFrameRefresh) capture(now time.Time, jpegMode, requested bool, capture, snapshot func() (image.Image, error)) (image.Image, error) {
	img, err := capture()
	if err == nil {
		r.next = now.Add(idleJPEGRefreshInterval)
		return img, nil
	}
	if !errors.Is(err, desktop.ErrNoNewFrame) {
		return img, err
	}
	if r.next.IsZero() {
		r.next = now.Add(idleJPEGRefreshInterval)
	}
	if requested || (jpegMode && !now.Before(r.next)) {
		r.next = now.Add(idleJPEGRefreshInterval)
		return snapshot()
	}
	return img, err
}
