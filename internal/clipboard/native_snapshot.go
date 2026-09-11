package clipboard

import "errors"

// readStableContent 不持有 Sync.nativeMu。延遲提供內容的 macOS Pasteboard
// 可能等待跨程序／跨裝置資料；接收端必須仍能發布新內容，解除相互等待。
// 系統在讀取期間更新序號時重新讀取；呼叫端提交狀態前仍須再次核對序號。
func readStableContent(read func() (content, error)) (content, int64, error) {
	for attempt := 0; attempt < 4; attempt++ {
		revision := nativeRevision()
		value, err := read()
		if err != nil {
			return content{}, revision, err
		}
		if revision == nativeRevision() {
			return value, revision, nil
		}
	}
	return content{}, nativeRevision(), errors.New("剪貼簿讀取期間已變更")
}
