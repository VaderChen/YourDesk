# YourDesk WinPE — 實驗性救援 Host

這是 WinPE x64 的實驗性便攜版本，只有編譯確認，尚未在真實 WinPE 驗證。請使用 Windows 10 / 11 核心、x64 架構且已具備網卡驅動的救援環境；首選有線網路。它不是一般 Client 安裝包，也不提供 BIOS / UEFI 操作。

## 啟動

1. 把整個目錄放到自行建立的 WinPE USB、映像或其他可存取位置。
2. 執行 `start-yourdesk.cmd`。在 WinPE 會先執行 wpeinit 初始化網路，再啟動原生視窗。
3. 視窗會顯示本次啟動的臨時 ID；按「Show / 顯示」查看臨時密碼，交給獲授權的近端使用者連線。
4. 近端使用一般 YourDesk Remote，**Tailcat 模式須關閉**。本版不支援 Tailcat，若近端要求此通道會明確失敗。
5. 按「Stop / 停止遠端連線」會停止接受連線並斷開現有工作階段；「Start / 開始接收」可重新開放。關閉視窗即結束 Host。

預設配對位址為 `wss://desktop.mars-cloud.com:8080/ws`。需要自訂時：

```bat
start-yourdesk.cmd -signal wss://your-server.example:8080/ws -fps 8 -quality 60
```

FPS 範圍 1–20、JPEG 品質 20–90。ID 與密碼只保留於記憶體，每次程式重新啟動都更換，不從 PE 映像讀取 MachineGuid，也不建立共用固定密碼。不要將畫面上的密碼寫入公開記錄。

## 本版能力

- 單一螢幕、GDI 擷取、軟體 JPEG、既有 WebRTC P2P 與密碼驗證。
- 原生 Win32 視窗，標示 Experimental / 實驗性，不需要 WebView2、Explorer 或 Tray。
- Tailcat 灰色停用，旁邊 `(?)` 泡泡顯示原因；建置不包含此後端，不宣告支援，也拒絕遠端自動協商。
- 剪貼簿、檔案傳送及遠端 Shell 停用；不安裝系統服務，不提供自動更新。
- 停止時回收通道；再次啟動會等待上一輪回收完成，避免建立重疊的 Host。

精簡 PE 可能缺少網卡驅動、必要 Win32 API、根憑證或中文字型。視窗提供英文操作標籤；若中文字顯示不完整，需在自行建立的 PE 加入語言／字型套件。連線失敗時應檢查 IP、DNS、時鐘與 WSS 憑證，不要停用 TLS 驗證。原生 UDP 被網路阻擋時，本版沒有 Tailcat 中繼可代替。

在 PE 的 Startnet.cmd 自動啟動時，可先定位本目錄，再呼叫本包啟動腳本；磁碟機代號不保證固定，不能直接假設 USB 或離線 Windows 一定位於 C 槽。停止或更新 Host 請重新替換此便攜包，不執行一般 Client 安裝程式。

WinPE 本身應用於部署與復原。此包不含 Microsoft WinPE 映像；請由合法的 ADK / WinPE 環境自行建立救援媒體。
