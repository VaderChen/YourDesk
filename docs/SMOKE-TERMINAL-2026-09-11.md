# 命令列與本機能力 Smoke Test（2026-09-11）

本輪使用 macOS 本機回環連線，沒有連接既有遠端站台或部署正式 Server。

| 項目 | 結果 |
| --- | --- |
| 真實 TLS 配對、P2P 控制通道、PTY | 通過 |
| 保留工作目錄、繁體中文與 Unicode | 通過 |
| Ctrl+C 中斷前景程序 | 通過 |
| PTY 尺寸調整為 101 × 37 | 通過 |
| 超過 400 KB、12,000 行輸出與背壓 | 通過，無缺行 |
| 重複輸入序號避免重複執行 | 通過 |
| 關閉終端機、突然斷線與重連後的 Shell 清理 | 通過 |
| 真實 Remote helper 與無桌面 Host 分支協商 | 通過 |
| 舊版缺少 terminal 能力時顯示更新提示 | 通過，沒有進入桌面／終端機 |
| xterm.js 中文／ANSI 渲染、鍵盤、resize、關閉 | 真實 Edge 瀏覽器通過；UI API 使用模擬回應 |
| 無桌面裝置的連線方式選擇 | 桌面停用、命令列保留 |
| Server 的 WSS／HTTPS 能力註冊與查詢 | 本機 TLS Server 通過 |
| 舊 Server 拒絕 details 後退回 bool 查詢 | 修正後通過 |
| 真實 Client join 的能力摘要與簽章 | 通過 |
| Linux 無 DISPLAY／WAYLAND_DISPLAY 的啟動選擇 | 分支測試通過 |
| macOS、Windows x64／arm64 Client／Remote | 編譯通過 |
| Linux x64／arm64 無 GUI Host | 編譯通過 |
| Windows x64、Linux x64／arm64 Server | 編譯通過 |

測試來源：`internal/terminal/smoke_test.go`、`cmd/remote/terminal_smoke_test.go`、`internal/clientui/presence_smoke_test.go`、`internal/signaling/host_binary_smoke_test.go`、`internal/runtimeenv/desktop_test.go`、`scripts/smoke-terminal-ui.cjs`。Server 測試位於其專案的 `internal/signaling/capabilities_test.go`。

尚未涵蓋 Windows ConPTY 執行、Linux 實機、跨機網路，以及完整正式 Client UI 到真實 Shell 的單一端到端測試；本輪分別驗證真實傳輸／Shell 與瀏覽器渲染。Server 修改尚未部署，GitHub 與 Release 未更新。


## 獨立視窗修正後補測

- 真實 macOS 原生 WebView：兩個視窗同時載入、個別關閉、父程序 stdin EOF 回收均通過。
- 真實瀏覽器 → 正式 UI API → Remote helper → TLS／P2P → PTY：兩個站台提示字元與輸入輸出通過，關閉一個後另一個仍可執行指令。本次沒有模擬 API 或 Shell。
- 舊視窗的 write／close／disconnect 請求會被連線識別拒絕，避免影響重新建立的連線。
- 連線方式選擇、無桌面能力停用及原生視窗啟動請求通過。
- macOS、Windows x64／arm64 Client／Remote 編譯通過。Windows ConPTY 的 Handle 與提示字元變更仍待 Windows 實機驗證。

新增重跑入口：`TestTerminalBrowserEndToEnd`、`TestNativeTerminalWindows`、`TestTerminalRejectsStaleWindow` 及 `scripts/smoke-terminal-e2e.cjs`。原生視窗測試須明確設定 `YOURDESK_TERMINAL_WINDOW_BINARY`，避免一般測試自動開啟視窗。
