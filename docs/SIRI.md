# Siri 與 Siri AI

YourDesk 的 macOS 發行包內含 App Intents Extension。四個操作共用既有 Client 連線流程，不依賴 MCP 開關。

## 支援範圍

| 功能 | Siri／捷徑（macOS 13 以上） | 新版 Siri AI |
| --- | --- | --- |
| 開啟 YourDesk | App Shortcut | 可使用同一捷徑；系統也能開啟 App |
| 連線到已儲存電腦 | App Shortcut，選擇站台 | 同一捷徑入口；尚無遠端連線專用 schema |
| 中斷對外連線 | App Shortcut，可指定站台 | 同一捷徑入口；尚無遠端斷線專用 schema |
| 查詢對外連線狀態 | App Shortcut，回傳文字及語音 | 同一捷徑入口；尚無遠端狀態專用 schema |
| 開啟指定站台 | 可透過捷徑選取電腦 | macOS 27 `.system.open`，開啟主畫面並篩選該站台 |

新版 Siri AI 的自由自然語言、跨 App 組合操作不能只靠自訂 AppIntent 宣告就保證可用。Apple 目前公開的 schema domains 沒有遠端桌面連線類別；本實作不將連線、斷線或查詢偽裝為其他類別。`.system.open` 只顯示站台，不自行建立遠端連線。後續 Apple 開放對應 schema 時可沿用本機操作介面。

## 使用

安裝完整 YourDesk.app 並開啟一次，在「捷徑」搜尋 YourDesk，可使用：

- 開啟 YourDesk。
- 使用 YourDesk 連線到「站台名稱」。
- 中斷 YourDesk 連線。
- 查詢 YourDesk 連線狀態。

Siri 的實際口令識別取決於系統語言、地區、Siri 設定及系統索引；也可以在捷徑中自訂名稱。名稱相同的站台由實體 ID 區分，系統可要求選擇。

連線僅接受已儲存站台及既有記住的密碼。未保存密碼時開啟主畫面並提示先完成一般連線；密碼不交給 Siri。回覆「已開始連線」不代表驗證或桌面串流已成功，須再查詢狀態。未指定電腦且有多條對外連線時，不會任意挑選或全部斷線，而是要求指定。中斷操作不停止本機 Host 或別人的入站連線。

## 本機介面與封裝

- Go Client 在原有 127.0.0.1 隨機埠上加入限定動作的 `/automation`；獨立隨機 token，不開放任意 API 或指令。
- `~/Library/Application Support/YourDesk/siri-bridge.json` 權限為 0600，含 URL、能力 token 及 PID；正常結束時移除，重啟後換新。
- 拒絕帶 Origin 的瀏覽器請求及錯誤 token；只接受 POST。Siri 回應不含站台密碼、備註或本機 UI token。
- macOS ExtensionKit 擴充位於 `Contents/Extensions/YourDeskSiri.appex`，使用 App Sandbox、網路 client 權限及僅讀取上述單一檔案的 home-relative exception。主程式仍採現有非沙盒架構，不額外要求 App Group。
- Swift 請求禁止 HTTP 重新導向與代理，限制到 127.0.0.1 的固定 `/automation` 路徑。四個操作要求本機裝置驗證。
- `scripts/siri.py` 編譯 Swift、抽取 metadata 並簽署擴充；`scripts/release.py` 在外層 App 簽署、公證前執行。需要 Xcode 27 SDK；舊系統保留 macOS 13 的 App Shortcuts，新 schema 用 availability 隔離。
- `runUITest.command --build-only` 只產生 Go 執行檔，Siri 必須使用完整 App bundle，不能以裸執行檔驗證系統索引。

## 驗證與限制

已加入本機橋接測試，涵蓋未授權來源、憑證權限、站台資料最小化、缺少密碼、正式連線 API 的子程序啟動／中斷、多連線歧義及憑證清理。連線測試使用無網路功能的子程序替身，不代表遠端桌面或語音端到端測試。

本機 macOS 26.6.2／Xcode 27 SDK 可編譯及抽取新版 schema，但不能驗證 macOS 27 Siri AI 的真實對話。發行前仍需在安裝後的「捷徑」與 Siri 上驗證擴充啟動、沙盒檔案存取、四個動作及 macOS 27 的站台開啟；目前不宣稱已完成語音端到端驗證。

## Apple 官方資料

- [AppIntent](https://developer.apple.com/documentation/appintents/appintent)
- [Apple Intelligence and Siri AI](https://developer.apple.com/documentation/appintents/apple-intelligence-and-siri-ai)
- [App schema domains](https://developer.apple.com/documentation/appintents/app-schema-domains)
- [System open schema](https://developer.apple.com/documentation/appintents/appschema/systemintent/open)
- [WWDC26：Build intelligent Siri experiences with App Schemas](https://developer.apple.com/videos/play/wwdc2026/240/)
