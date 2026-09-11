# WinPE 實驗性救援 Host 實作

本版已實作並完成編譯與打包，尚未在實機 WinPE 驗證。目標為 Windows 10 / 11 核心的 WinPE x64；不宣告所有 PE 映像皆相容。

## 已完成

- `cmd/winpe`：獨立原生 Win32 視窗，標題與功能標示 Experimental / 實驗性。每次程式啟動產生隨機臨時 ID 與 16 字元密碼，密碼預設遮蔽，只在視窗顯示，不讀取映像 MachineGuid、PowerShell 或 CIM。
- `internal/hostsession`：由一般 Client 抽出的共用 Host 核心，保留既有驗證、WebRTC、畫面封包、鍵鼠與中斷流程；一般 Client 也改呼叫此核心。救援入口關閉剪貼簿與遠端資料／Shell 註冊，限制單一螢幕。
- Windows 無 CGO 建置：使用現有 GDI 擷取、Go JPEG 與 SendInput；排除 DXGI / Media Foundation 的 cgo 實作。救援入口拒絕未加 winpe 標籤的建置，亦不提供 CGO 入口。
- Tailcat：winpe build tag 使用空的虛擬後端清單，不編入 Tailcat / Tailscale。原生視窗以灰色、未勾選的控制項呈現，旁邊 `(?)` 泡泡解釋原因。CLI、能力宣告與單端自動協商均使用同一後端清單，因此無法繞過。
- 連線控制：Start 開放接收；Stop 同時取消配對等待與現有連線，等待前一輪回收後才可重新啟動。視窗關閉會停止 Host；命名 mutex 防止同一登入工作階段重複啟動。
- 包含網路初始化腳本、繁中使用說明與編譯相依的授權文件。配對伺服器可由 CLI 指定，預設使用既有 WSS 位址。

說明放在原生 tooltip，避免視窗堆疊副標；英文操作標籤供缺少中文字型的精簡 PE 使用。

## 建置及產物

一般 `pack.command` 預設目標已包含 `winpe/amd64`；`buildWin.command` 也會建置 WinPE，接著使用 `pack.command --no-build` 封裝。WinPE 固定提供便攜 ZIP，不使用 Installer；一般 Windows amd64 / arm64 仍使用既有 Installer。

一般打包產物：`dist/winpe-amd64/YourDesk-{版本}-winpe-amd64-experimental.zip`，與其他平台共用同一版本號。也可以設定 `YOURDESK_BUILD_TARGETS=winpe/amd64` 後執行 `pack.command`，只建置並封裝 WinPE；此入口依既有規則清空 `dist`。不需 NSIS 或 MinGW。

以下獨立入口共用 `scripts/release.py` 的編譯、授權與 ZIP 封裝邏輯，不清理一般 `dist`：

```sh
./scripts/build-winpe.sh
```

產物：`.local-run/winpe-dist/YourDesk-WinPE-x64-experimental.zip`。

腳本使用 `CGO_ENABLED=0 GOOS=windows GOARCH=amd64`、`-tags winpe`、`-mod=readonly`，並以 Go 套件相依清單阻擋 WebView2、clientui、winmedia 及 Tailcat / Tailscale 混入。ZIP 完成後才替換舊 ZIP，暫存檔自動清理，不覆寫正在執行的 EXE。

解壓後於 PE 執行 `start-yourdesk.cmd`。腳本先執行 wpeinit，再啟動 Host；顯示臨時 ID 與密碼後，由一般 YourDesk Remote 連線。近端須關閉 Tailcat，本版只接受原生 P2P。

## 驗證範圍

已完成 WinPE x64 編譯、套件相依盤點、ZIP 內容確認，以及抽出核心後的一般 macOS Client / Remote 編譯。沒有啟動救援 EXE、建立 PE 映像、執行網路或畫面互連測試，亦未更新正在執行的 App、推送 GitHub 或發布 Release。

必要 API 的啟動檢查不代表全部 runtime 相依已確認；真實 PE 仍需檢查網卡、DNS、時鐘、CA、GDI / 輸入桌面及 Win32 控制項。Tailcat 暫時維持不支援，而非因 Windows 編譯成功便啟用。

## 目前限制

剪貼簿、檔案傳輸、MCP / 遠端 Shell、硬體影像、ARM64 套件、持久 ID、自動更新及完整 PE 映像製作不在本版範圍。網路阻擋原生 UDP 時沒有 Tailcat 備援；不能把 PE 已啟動後的遠端救援當成 BIOS 或冷開機控制。
