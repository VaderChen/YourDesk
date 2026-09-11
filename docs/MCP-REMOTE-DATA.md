# MCP 遠端資料與 Shell

Agent 先透過 `connect` 建立一般密碼驗證連線，以 `get_status` 取得 connected 工作階段，將其 `session` 傳給下列工具。資料由遠端 Host 經既有可靠的 P2P control DataChannel 回傳，不在近端代為讀檔或執行 Shell，也不新增 Server 管理指令或監聽埠。

| MCP 工具 | P2P 指令 | 參數與結果 |
| --- | --- | --- |
| get_remote_filesystem | files.roots | session；回傳遠端家目錄與限制 |
| search_remote_files | files.search | session、path、query、content、limit、depth；回傳 entries、visited、skipped、truncated |
| read_remote_file | files.read | session、path、offset、length；回傳 Base64 data、nextOffset、eof、size、modified |
| run_remote_shell | shell.run | session、command、directory、timeoutMs；回傳合併 output、exitCode、timedOut、truncated、shell |

搜尋 query 比對檔名、不分大小寫；content 比對 UTF-8 文字、區分大小寫，只讀每檔前 64 KiB。預設最多 20 筆、深度 4，最大深度 8；一次最多查看 5000 筆，約 2.5 秒期限。遇到限制會標示 truncated，請縮小 path 或條件後再查。檔案讀取每次最多 4096 bytes，可用 nextOffset 續讀；檔案變動時應重新開始，不能把不同 size／modified 的區塊直接拼接。二進位資料亦可讀取，但不解析 PDF、Office 或圖片內容。

搜尋／讀取以 `os.Root` 限制在遠端家目錄，拒絕路徑跳脫、符號連結與特殊裝置，不寫入檔案。一般 OS／網路檔案系統的底層 I/O 可能無法立即中斷；搜尋期限在各步驟檢查，並非硬性中斷所有系統呼叫。

Shell 為獨立、非互動指令：macOS 使用 `/bin/sh -c`，Windows 使用系統 `cmd.exe /D /S /C`；可在明確指令中呼叫 PowerShell。工作目錄預設遠端家目錄，也可指定遠端絕對路徑。執行時間預設／最多 3000 毫秒、最少 100 毫秒；合併輸出最多保留 2048 bytes，超量持續排空但標示 truncated，非 UTF-8 輸出使用替代字元。exitCode 由 OS 回傳，逾時以 timedOut 為準。每次呼叫不保留前次的工作目錄或 Shell 變數。

Shell 使用 Host 帳號權限，可修改資料；檔案工具的家目錄範圍不是 Shell 沙箱。macOS 以程序群組管理，Windows 在程序開始執行前加入 Job Object；逾時、P2P 斷線或指令結束後清理所管理的程序。不要用於啟動長駐服務或刻意脫離程序群組的工作。root Host（包含未登入服務工作階段）不公告這四項能力。

遠端顯示停用控制時拒絕新的資料指令，每個遠端顯示最多並行兩項。已在遠端執行的指令由其期限／斷線處理；取消 MCP HTTP 等待不保證立即停止遠端指令。結果未知時，不可自動重送具有副作用的 Shell 指令。

MCP 白名單啟用時，名單內免 Token、名單外拒絕；關閉時所有來源需 Bearer Token。遠端連線仍需正常密碼驗證。Token、Shell 命令及回傳內容不另寫入診斷 LOG；Agent 的對話／工具記錄仍可能保存呼叫資料。舊版 Host 未公告能力時明確回報不支援，不降級成鍵盤輸入或近端 Shell。

本次僅完成語法與編譯檢查，未執行額外測試、遠端 Shell 或實機資料讀取。此文件描述本機原始碼功能，不代表已打包或發行。

## 互動命令列與站台能力

`get_site_capabilities` 查詢已儲存站台的在線狀態與桌面、命令列、剪貼簿能力，以站台 ID 為鍵。缺少能力欄位代表未知；舊 Server、IP 直連或網路失敗時，不應當成不支援。

使用 `connect` 加上 `terminal: true`，可連線已儲存站台（id）或裝置（room），包含支援命令列的無桌面系統。不可搭配 diagnostics。以 `get_status` 等待 connected，保存該筆 session 與 instance，再呼叫 `remote_terminal`：

| action | 使用方式 |
| --- | --- |
| open | 建立 PTY；columns、rows 預設 80、24 |
| read | 首次 ack=0；處理輸出後，以回傳的 sequence 作為下一次 ack。data 是 Base64 原始資料，包含 ANSI 控制碼；空資料不代表結束 |
| write | text 為原始輸入，Enter 使用 `\r`、Ctrl+C 使用 `\u0003`；每筆最多 2048 bytes，sequence 從 1 遞增 |
| resize | columns 20～500、rows 5～300 |
| close | 結束命令列及其遠端連線 |

每次操作都需 session、instance、action。重連後必須重新取得 instance；MCP 不共用使用者已開啟的終端機視窗，以免互相消耗輸出。read 收到 ended 時仍須處理該筆 data。輸入逾時若需重試，沿用原 sequence 與內容，不可把同一命令以新序號重送。終端機詢問游標位置時，Agent 須以 write 回覆，例如 ESC[6n 可回覆 ESC[1;1R；這是原始終端機協定，不是已渲染的文字畫面。

互動模式保留工作目錄及變數，權限與遠端使用者相同；關閉或斷線時回收 Shell。既有 `run_remote_shell` 是獨立短指令，不保留狀態，與檔案工具目前仍使用一般桌面連線。舊版不支援命令列時會提示更新，不改在本機執行。

`get_preferences`／`set_preferences` 也支援 `closeWindowOnDisconnect`（預設 false），控制桌面與命令列視窗在斷線後是否自動關閉。
