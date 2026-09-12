package hostsession

import (
	"image"
	"yourdesk/internal/p2p"
)

// 擷取取得獨立影像；縮圖及編碼資源只由編碼階段持有。
type capturedFrame struct {
	Image   image.Image
	Display int
	Epoch   uint64
}

// 一次擷取產生的 JPEG 區塊整批交付，避免跨影格交錯。
type encodedFrames struct {
	Frames          []p2p.Frame
	Epoch, Recovery uint64
	BitrateLimit    int // 編碼當下的傳送上限快照，避免跨工作者讀取設定。
}
