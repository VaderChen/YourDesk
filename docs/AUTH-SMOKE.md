# 密碼握手本機 Smoke（2026-09-10）

在 macOS 使用測試密碼與本機 TLS WebSocket 執行，未讀取、輸出或變更使用者密碼。測試只信任本機 httptest 憑證，未修改正式 TLS 驗證。

三輪測試全部通過，Go race detector 未發現競態：

- 正確密碼驗證 offer，並簽署 answer 由本機對端驗證。
- 一般、中文、前後空白及舊 Base64 格式的測試密碼。
- 錯誤密碼後透過正式 ResolveSecret 流程改輸正確密碼。
- 過期 offer 被略過，後續有效 offer 正常驗證。
- 錯誤密碼拒絕連線；被修改的 payload 無法通過簽章驗證。
- 密碼經 JSON 標準輸入管道傳入，LF 與 Windows CRLF 結果一致，中文及尾端空白保留。

```sh
go test -race ./internal/security ./internal/signaling -run 'TestAuthenticationSmoke|TestSignVerify|TestGeneratedSecret' -count=3 -timeout=45s -v
go test -race ./cmd/remote -run TestPasswordPipeSmoke -count=3 -timeout=30s -v
```

本次驗證正式密碼解碼、signaling 收送／簽章與 stdin 讀取流程，未啟動完整 Client UI，也未經正式中央服務、Windows 程序或使用者實際設定。尚未重現兩端同為 1.26.0910 build 0858、新版被控端驗證失敗但舊版可連線的現場問題；不能以這些測試宣稱該問題已修復。

## 指定遠端與啟動管道檢查

經使用者授權，以臨時密碼測試指定遠端，收到約 30 秒內的有效時效 host offer，但 HMAC 不符，已重現現場失敗。常見 SDP JSON 順序／跳脫及舊金鑰衍生變體皆未通過。使用者確認完整重啟亦無效，且上一版可連線；尚未證明根因。

發現 UI 與被控子程序各自讀取密碼檔，與程式原有「使用介面顯示的配對資訊」意圖不一致。改為啟動 host 時加上 -secret-stdin，透過私有 stdin JSON 傳入 UI 已載入的密碼；不把密碼放進命令列或紀錄。此修正消除兩次載入不同來源的可能，不能逕自視為現場根因已確認。

新增程序層級 TestHostSecretPipeSmoke，使用正式 server.start 啟動本機測試子程序，驗證中文與尾端空白密碼經管道完整抵達，三輪 race 測試通過。Windows x64／ARM64 修正版供被控端驗證；尚待實機重測。

## 真正 Host 執行檔測試與跨版本矩陣

使用者確認「新連新失敗、新連舊成功、舊連新失敗」，因此目前優先追查新版被控端。傳入密碼的驗證版未解決現場問題，不視為根因修復。持續監看指定遠端仍收到簽章不符的 offer，尚未取得可通過的握手。

新增 TestHostBinaryAuthenticationSmoke：啟動真正 yourdesk-client 執行檔，用測試憑證建立本機 TLS WebSocket 服務，驗證其 join 與 offer；不傳送 answer。macOS 本機測試已通過。Windows x64／ARM64 提供獨立測試工具與 CMD，使用固定測試密碼，不讀取使用者設定密碼，用以區分 Windows Host 本機簽章問題與正式服務／現場程序問題。

## Windows LOG 診斷版

提供 opt-in 建置旗標 `-X yourdesk/internal/authlog.Enabled=1`，正式版預設不寫 LOG。檔案位於作業系統 UserCacheDir 的 YourDesk/Logs；Windows 為 `%LOCALAPPDATA%\YourDesk\Logs`，UI 與 host 依程序各自寫入 JSONL，單程序檔案上限 5 MiB。

紀錄版本、程序關係、密碼來自 stdin 與否、解碼成功與否、TLS signaling 連線、join／offer 簽章序列化前後自我驗證、訊息送出結果及收到訊息的驗證分類。UI 另透過私有管道的一次性隨機 challenge，確認 host 解碼出的密碼是否與 UI 相同，只將布林結果寫入 LOG；challenge／proof 不寫入 LOG。不記錄密碼、金鑰、簽章、SDP、遠端桌面或剪貼簿。

Windows x64／ARM64 診斷版編譯通過；macOS 診斷執行檔的實際 join／offer 本機驗證通過，並確認 JSONL 事件正常產生。此版用於蒐集 Windows 現場證據，尚未宣稱根因已修復。

## 目前狀態

後續對照確認占用連線仍持續送出 Offer，且至少一次指定密碼驗證成功；Windows 查詢僅見正常的 APP／Host 父子程序，尚不能將原因歸結為本機殘留。新版的啟動前查詢與確認停止措施不是完整修復。部分電腦仍可能無法被連線，目前建議重新啟動該電腦後再試，問題持續處理中。
