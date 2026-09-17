# Mac 顯示區域鍵鼠無法控制的診斷

目前回報：Mac 筆電剛連上 Windows 即無法使用鍵鼠，但影像正常；桌上型 Mac 連同一台 Windows 正常，Fn＋F12 無法恢復。原因尚未確認，不能直接歸因於 Windows 休眠或觸控板座標。

開啟「封包分析」後，顯示區域每 5 秒產生一筆 `viewer-input` 事件；關閉時不記錄。日誌位於 macOS 的 `~/Library/Caches/YourDesk/Logs/`。應取得故障筆電的 viewer 日誌，不能以另一台 Mac 的紀錄代替。

- `focused`、`controlEnabled`、`popupOpen`、`cropBlocksInput`：確認本機是否允許控制。
- `displayInputReady`、`displayPending`、`displayCount`、`display`／`displayedDisplay`、`viewID`／`displayedViewID`、`viewChanging`：確認輸入是否因螢幕或裁切切換而暫停。
- `rawSupported`、`rawAvailable`、`rawActive`：確認原始鍵盤擷取是否啟用。
- `queuedMove`、`queuedButton`、`queuedKey`、`queuedWheel`：這段取樣期間交給輸入佇列的數量，**不代表封包已送出或 Windows 已執行**。
- `queueFailed`：交給輸入佇列失敗的次數。

測試時將顯示區域取得焦點，移動滑鼠並按幾個無敏感內容的測試按鍵，持續至少 10 秒。比對同時段控制通道流量與 Windows Host 的輸入錯誤紀錄，再判斷問題位於本機攔截、佇列、傳輸或遠端注入。日誌不記錄按鍵內容、游標座標或剪貼簿內容。

此變更只增加診斷，尚未宣稱修復上述問題。
