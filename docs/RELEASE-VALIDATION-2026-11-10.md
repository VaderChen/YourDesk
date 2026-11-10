# 時鐘差相容性驗證（2026-11-10）

對應版本：1.26.1110 build 1451。使用者確認原本 Windows 連 Mac 的命令列在同步兩端系統時間後恢復正常，支持舊版絕對期限造成拒絕的判斷。

## 已完成

- `go test -mod=readonly ./internal/p2p ./internal/terminal ./cmd/remote` 通過。
- `android/core` 的 `go test -mod=readonly ./...` 通過。
- 真實 WebRTC DataChannel：模擬正負 24 小時時差，新版請求與回覆成功，handler 使用接收端有效期限；重複 ID 取回同一結果而不重複執行。
- 舊版未公告能力時不傳相對預算，保留絕對期限驗證；明確過期、過長或無效預算仍拒絕。
- 處理中重複請求不延長 handler 期限；期限屆滿或 Peer 關閉會取消 context。
- `YOURDESK_TERMINAL_SMOKE=1 go test -mod=readonly ./internal/terminal -run '^TestLocalP2PTerminalSmoke$' -count=1 -v` 通過真實 TLS／P2P／PTY、中文輸出、工作目錄、resize、Ctrl+C、超過 400 KB 輸出、關閉及重連。
- Windows x64／ARM64 的 P2P 指令模組交叉編譯通過。這是編譯檢查，不是 Windows 實機驗證。

## 限制與發行檢查

新版尚未在原回報的 Windows／Mac 兩台實機安裝驗收。舊版同步時間後成功不等於新版跨機驗證完成。新流程需雙端支援；混用舊版仍須同步日期與時間。呼叫端等待與接收端執行預算分別計時，網路延遲仍可能造成結果未確認，不自動重送副作用指令。

正式發布前須通過所有平台打包、Windows Portable／服務 ZIP 版本與架構核對、Android 正式簽章與版本碼／來源核對，以及 macOS App／DMG 簽章、公證、票證和 Gatekeeper 檢查。發行使用 16 個附件與 SHA-256 清單，發布後從 GitHub 讀回大小、雜湊、標籤與四語內文核對。機器檢查證據保存在本機 `.local-run/release-1.26.1110-build-1451/`，不將憑證或暫存備份提交至原始碼。
