# 站台 QR Code

本機 ID 右側、複製按鈕旁新增 QR Code 圖示。按下後在本機產生圖片，顯示站台名稱與 ID，亦可複製站台連結；不透過外部 QR 服務。

本次只處理桌面端，手機 App 的掃描與站台匯入尚未實作。QR Code 不包含連線密碼或管理介面 token，也不授權連線。未支援此格式的手機 App 無法直接新增站台。

## 資料契約 v1

```text
yourdesk://site?v=1&name=Office&room=YD-XXXX-XXXX-XXXX-XXXX-XXXX&signal=wss%3A%2F%2Fexample.com%2Fws
```

- `v`：固定為 `1`，供手機端辨識格式版本。
- `name`：本機名稱，空白時使用 ID。
- `room`：本機裝置 ID。
- `signal`：目前使用的信令伺服器 URL。僅支援現有桌面端可用的 ws／wss／https 格式，含使用者資訊、query 或 fragment 時拒絕分享，避免將 URL 中的認證資料帶入 QR。

欄位依標準 URL query 編碼，讀取端應使用 URL 解析器還原，勿直接分割或以 HTML 插入名稱。未來手機匯入端應驗證格式及欄位，讓使用者確認後儲存；連線密碼另行輸入。

桌面端 `/api/site-qr` 沿用既有 UI token 與來源檢查，回傳 `name`、`room`、`uri` 與 PNG data URL `image`，設定 `Cache-Control: no-store`。圖片與連結在關閉對話框後清除；過期回覆不再更新視窗。

使用 [go-qrcode](https://github.com/skip2/go-qrcode) 產生具標準白邊的 512 × 512 PNG（Medium 錯誤更正），固定依賴版本並隨套件提供 MIT 授權。

Smoke：驗證授權、中文／特殊字元往返、欄位白名單及 PNG；使用 macOS Vision 實際解碼並比對 URI；以瀏覽器驗證按鈕、視窗、關閉清除及延遲回覆。這不代表手機匯入已完成。
