# 待發行修正

## 串流穩定問題已處理

- 已處理解碼佇列滿載時阻塞 WebRTC DataChannel 接收回呼的問題；目前回報只發生在 macOS 對 macOS，其他平台尚未觀察到相同問題。
- 新增非阻塞 `TrySubmit`，一般 Viewer 與背景診斷均改用此入口；佇列滿載時略過新影格，保留已排隊影格順序及既有解碼恢復流程。
- 加入佇列滿載及連續 100,000 次交接的高壓回歸測試。
- 本次未加入背景自動重連／斷線續傳。

程式修正已同步 GitHub 主分支（[`754660b`](https://github.com/VaderChen/YourDesk/commit/754660b3dec39bed957997ab9fe9f7c6727ce236)）。已發布的 [build 2323](RELEASE-1.26.0913-build-2323.md) 不包含這筆後續修正；本次同步不替換其套件。詳見 [串流穩定修正與存活判斷](STREAM-RECOVERY.md)。
