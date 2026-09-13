# 訊號心跳 v2

Client 預設在 WSS Upgrade 與 HTTPS POST `/signal/session` 請求 `YourDesk-Heartbeat-Protocol: heartbeat-v2`。Host 與 Viewer 各自協商，無須雙端同時升級。

只有成功握手（101／201）回覆相同協議、`YourDesk-Heartbeat-Interval: 60` 及有效 timeout 才啟用 v2。WebSocket timeout 固定為 10 秒 Pong 等待；HTTPS timeout 為 180 秒活動期限。這些是目前 v2 的固定契約，不接受任意伺服器數值作為計時器。

未接受、未知協議、缺少或不符時序的回覆都保留 v1。HTTPS v1 每 15 秒送心跳；v2 每 60 秒送心跳，兩者的 HTTP 心跳請求期限均維持 5 秒。暫時網路錯誤留待下一次重試；401／410 代表 session 已失效，取消進行中的讀寫，交由既有上層流程處理重連。

WebSocket v2 使用有界背景讀取，持續處理 Ping/Pong，避免密碼輸入期間或握手完成後無 Receive 導致誤判。最多暫存 16 份訊息，滿載時關閉訊號連線，避免無界記憶體；JSON、room、binding、時間與 HMAC 驗證仍由原本 Receive 路徑處理。直連 TLS 不採用中央 Server 的心跳協商。

Join／Offer／Answer、HTTPS token／sequence／ack 的線上格式與密碼驗證不變；v2 不要求 join capabilities 宣告。`signaling-connected` 診斷事件包含實際採用的 heartbeatProtocol，HTTPS 另有 heartbeatIntervalSeconds。

## 驗證

2026-09-13：

- Client 對支援及忽略 v2 的 TLS 測試 Server 完成雙向握手。
- 缺少、未知與異常協商值保守回退。
- WebSocket v2 在應用層未呼叫 Receive 時仍能回 Pong。
- 模擬時間驗證 HTTPS 15／60 秒計時、暫時失敗重試及 session 回收後停止。
- 更新後的正式 Client Dial 程式對獨立啟動的 YourDeskServer 工作目錄版本完成 WSS↔WSS、WSS↔HTTPS、HTTPS↔WSS、HTTPS↔HTTPS 四組配對；雙方確認 heartbeat-v2，Offer／Answer 經 HMAC 驗證。

```sh
go test -race ./internal/signaling -count=1
# 對專用測試 Server 執行；不可把正式環境當成測試站台。
YOURDESK_V2_SMOKE_URL=https://測試主機 YOURDESK_TLS_CA=/測試憑證.pem go test -race ./internal/signaling -run TestV2ServerIntegration -count=1
```

此為本機協議移轉及 Smoke 結果，不代表正式服務已部署或所有歷史安裝版皆已做實機驗證。使用更新後的執行檔才會送出 v2 協商；已安裝舊版本仍維持 v1。
