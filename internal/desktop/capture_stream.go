package desktop

import "errors"

// ErrNoNewFrame 是靜止桌面的正常狀態，不可當成擷取失敗或重送舊幀。
var ErrNoNewFrame = errors.New("畫面沒有新更新")

type LiveCapturer interface {
	Capturer
	Count() int
	Close()
}
