# HTTPS 初期訊號備援

新版 Client 在 WSS 建立失敗後自動改用同一主機的 HTTPS 8081，WSS 建立等候上限為 5 秒。此通道只用於配對和握手，不承載桌面影音，仍使用原有訊息簽章、TLS 信任與房間驗證。

若需直接使用 HTTPS，將站台的配對網址設定為 `https://desktop.mars-cloud.com:8081`。不同站台使用不同埠時，請各自填寫完整 HTTPS 網址。若仍使用 WSS 優先策略，可用 `YOURDESK_SIGNAL_HTTPS_PORT` 指定備援埠，預設 8081。

Server 必須同步升級為支援此通道的版本，並放行對應 TCP 埠。Server 的「設定 → 網路設定」修改埠後，Client 位址需同步調整。只允許 443 的網路仍需另外放行 8081。

HTTPS 傳輸使用建立工作階段的 POST、POST 傳訊、GET 長輪詢與心跳；權杖不置於網址、不跟隨重新導向。握手結束時刪除工作階段，失聯時由 Server 租期回收。WSS 與 HTTPS 共用配對狀態，可跨傳輸配對。健康檢查與裝置在線查詢也使用同一備援規則。

既有安裝版 Client 不會因為更新 Server 而自動取得此功能，需另外重新建置及安裝新版 Client。

## Client 接入範圍

一般 Client Host、Remote Viewer、未登入 Host 與 WinPE 共用 `signaling.Dial`，均適用此備援；Tailcat 與原生 P2P 的協商也沿用相同訊號通道。WSS 建立失敗才改用 HTTPS；既有連線中斷後由呼叫端重連，並非將進行中的握手無縫搬移。

HTTPS 工作階段建立上限 10 秒，長輪詢上限由 Server 設為 20 秒，Client 每 15 秒送出心跳。關閉會取消待處理的輪詢、傳訊及心跳，另以最多 3 秒的 DELETE 回收工作階段；重複關閉不重複送出。POST 使用相同序號重試，GET 使用接收確認，保留原封包與簽章。診斷事件記錄所選通道，不記錄工作階段 Token。

本次依本機 Server 的 `internal/signaling/http_transport.go` 核對協定並進行編譯確認；尚未進行實際網路阻擋、跨通道配對或 WinPE 實機驗證。

## 正式服務 HTTPS 驗證（2026-09-11）

使用正式主機 `desktop.mars-cloud.com:8081` 與獨立隨機測試房間，未連接或操作使用者桌面：

- HTTPS 健康檢查回傳 204，TLS 1.3 與憑證驗證成功。
- HTTPS Host／Viewer 雙向 Offer、Answer 及 HMAC 驗證成功，特殊字元與繁中 payload 保持一致。
- HTTPS Host 搭配 WSS Viewer、WSS Host 搭配 HTTPS Viewer 均成功。
- 指定同主機未提供 WSS 的埠，Client 建立失敗後自動改用 HTTPS 8081，完成訊號交換。
- Host 送出 Offer 後閒置 46 秒，仍可查得待機狀態並完成配對；工作階段關閉請求成功。

邊界觀察：若 Host 建立後未送出任何 Offer，等待超過 45 秒才送出，當下 presence 會顯示不可用。本機 Server 程式顯示，此時尚未開始更新 lastPong，需等下一輪心跳更新。正常先送 Offer 的待機流程驗證通過。此測試僅驗證訊號通道，不代表桌面影音或檔案傳輸實測。
