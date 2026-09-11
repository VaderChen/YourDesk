# 遠端命令列

站台右側的終端機圖示可開啟命令列，螢幕圖示可開啟桌面；雙擊站台後再選擇連線方式。兩端都須使用包含此功能的新版 YourDesk，並通過原有連線密碼驗證，不需要啟用 MCP。

終端機使用 xterm.js 顯示，可輸入指令、切換目錄及操作互動程式。Mac 使用目前帳號的 Shell，Windows 預設使用 CMD；關閉終端機視窗或中斷連線會結束該工作階段。每個終端機使用獨立原生視窗，不阻擋站台列表；不同站台可同時開啟。同一站台一次使用一種連線方式。

## 實作

- 本機 UI 使用 `@xterm/xterm 6.0.0` 與 `@xterm/addon-fit 0.11.0`，程式、CSS 與 MIT 授權檔一併內嵌，不依賴 CDN。配合 xterm.js 的動態樣式，內部網頁允許 inline style，JavaScript 仍只允許同源載入。
- Mac/Linux 使用 PTY，Windows 使用 ConPTY；Windows 缺少 ConPTY、WinPE，以及 root／系統服務執行環境不提供此功能。這次沒有實作登入畫面的系統權限 Shell。
- 使用既有加密 P2P 控制通道傳送 `terminal.open/read/write/resize/close`，由雙方交換能力清單，不另開 Shell 服務埠。傳輸沿用既有連線後端。
- 每個已驗證的 Peer 管理獨立終端機。輸入以序號防止重複，輸出使用確認序號；輸出緩衝區滿時等待消化，xterm.js 完成寫入後才讀下一批。
- 終端機建立後暫停桌面擷取；近端不建立桌面視窗、解碼器或剪貼簿同步。遠端仍沿用 Host 工作階段初始化。
- 斷線會關閉 PTY／ConPTY；Unix 結束仍在執行的 Shell 程序群組，Windows 透過 Job Object 結束程序樹。刻意脫離 Unix 程序群組的背景程序不保證一併結束。
- 另提供 MCP 互動終端機工具，詳見 MCP 文件。遠端輸出直接交給終端機解析，不插入 HTML，也未啟用連結或剪貼簿 addon。

## 驗證範圍

完成 JavaScript 語法與翻譯 JSON 檢查，以及 macOS、Windows x64／arm64 的 Client／Remote 和 WinPE 編譯檢查。已通過 macOS 本機對本機的真實 P2P／PTY Smoke Test，以及 Remote helper 的無桌面 Host 協商與舊版提示測試。已完成真實瀏覽器 → 正式 UI API → Remote helper → P2P → PTY 的兩站台端到端測試，確認提示字元、指令輸入輸出與個別關閉；另以兩個真實 macOS WebView 子程序驗證獨立視窗及父管線斷線清理。尚未進行跨機、Linux 實機或 Windows ConPTY 執行測試。

參考：[xterm.js addons](https://xtermjs.org/docs/guides/using-addons/)、[xterm.js flow control](https://xtermjs.org/docs/guides/flowcontrol/)。

## 啟動時偵測與能力查詢

維持同一個 Client 入口。Linux 啟動時若沒有 `DISPLAY` 與 `WAYLAND_DISPLAY`，自動略過 `-ui`，以無桌面模式待命；該分支不建立擷取器、圖像編碼器、滑鼠鍵盤控制或剪貼簿同步。現階段 root／系統服務帳號仍不提供互動 Shell，應以一般使用者執行。

Host 透過 WSS 或 HTTPS 備援的 join payload 回報 `schema: 1`、OS、架構、版本，以及 `desktop`、`terminal`、`clipboard` 三項能力。Server 僅將資料附在目前 Host 連線上，斷線即回收。這些是 Host 自述的公開摘要，不是權限驗證；真正的操作仍須通過 P2P 配對驗證。

Client 以 `POST /presence` 傳入 `{"rooms":["裝置 ID"],"details":true}` 查詢在線狀態與能力。無桌面裝置的桌面／畫面偵測按鈕會停用，命令列依對端能力決定；未知能力保留按鈕，連線後再協商。Server 未升級而拒絕 details 時，退回原本的在線狀態查詢。直接 IP 連線沒有 Server 能力摘要，連線後從 P2P 收到無桌面提示。

未帶 details 的 Server 查詢仍回傳原本的 `裝置 ID → bool` 格式。此變更已在本機修改 Server 原始碼，尚未部署到正式服務。

重跑測試：

```sh
YOURDESK_TERMINAL_SMOKE=1 go test ./internal/terminal -run TestLocalP2PTerminalSmoke -v -count=1
go build -o /tmp/yourdesk-terminal-smoke-remote ./cmd/remote
YOURDESK_TERMINAL_SMOKE_BINARY=/tmp/yourdesk-terminal-smoke-remote go test ./cmd/remote -run TestTerminalHelperSmoke -v -count=1
```


## 獨立視窗與 Windows 輸入修正

終端機頁面不再載入主介面 dialog／表單樣式，也不共用全域終端機狀態。每個視窗經私有 stdin 管線取得本機頁面位址，使用獨立連線識別驗證讀寫與關閉；舊視窗不會操作或中斷新連線。視窗關閉後管理程序回收對應 Remote helper，管理程序退出後 stdin EOF 也會關閉視窗。

在接收第一批輸出前先安裝 xterm.js 的輸入與終端機查詢回應通道，字型與視窗尺寸準備好後才建立 Shell。收到可見輸出才顯示已連線；十秒內沒有輸出會提醒檢查遠端 Shell，不以空白畫面當成就緒。

Windows 調整為明確指定標準 Handle，避免 CMD 沿用 Host 的私有 stdin／stdout 管線；CMD 使用 `/D /Q /K prompt $P$G` 設定提示字元。Windows x64／arm64 已通過編譯，尚未在該 Windows 裝置重現並驗證修正結果，不能以 macOS 測試取代 ConPTY 實機驗證。


## 斷線後關閉視窗

「設定 → 一般設定 → 調整畫面滿版」下方新增「斷線後自動關閉視窗」，預設關閉。關閉時保留桌面最後畫面或終端機輸出供查看；開啟時斷線會關閉對應視窗，命令列正常結束也會關閉。使用者主動關閉視窗及主程式退出仍會清理連線，不受此設定影響。
