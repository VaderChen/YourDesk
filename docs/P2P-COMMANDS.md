# P2P 端點交流指令

本文件盤點的是兩個已完成配對驗證的互聯端點，與 Server 管理指令、MCP 工具或本機 stdin 指令不同。新增指令沿用既有可靠、有序的 WebRTC control DataChannel，不另開埠，不讓 signaling Server 代為執行。

## 現有功能盤點

| 類別 | 現有訊息／機制 | 處理方式 |
| --- | --- | --- |
| 滑鼠、鍵盤 | move、button、key、wheel、raw-key、raw-key-reset | 保留即時事件，不為每次滑鼠移動增加 RPC |
| 螢幕 | displays、display-next、display-select | 保留螢幕與 displayRequest 對應 |
| 影片 | video-capabilities、video-status、keyboard-capabilities | 保留編解碼、版本與來源能力公告 |
| 串流配置 | stream-config、stream-config-result、quality-profile | 保留已有的工作階段、revision、結果回覆與舊版相容 |
| 剪貼簿與檔案 | hello、begin、ack、cancel、end、offer-start／entry／end、pull-read、pull-error | 保留獨立 clipboard DataChannel 與現有傳輸流程 |
| 連線生命週期 | Peer 關閉、ICE 狀態 | 加入明確的遠端斷線請求，原本關閉處理仍保留 |

## 新實作的共用指令

| 指令 | 支援端 | 回覆與用途 |
| --- | --- | --- |
| capabilities.get | 雙端 | 取得此端實際註冊的指令版本與名稱 |
| ping | 雙端 | 回覆遠端時間；呼叫端用自身經過時間衡量指令往返，不能用雙端時鐘相減推算單向延遲 |
| session.status | 雙端 | P2P 是否建立、累計傳送／接收位元組，不含密碼 |
| session.disconnect | 雙端 | 接受斷線請求，嘗試送回結果後關閉本次 Peer，不停用 Host 或關閉 OS |
| input.reset | Host | 清除本次工作階段追蹤的鍵盤、原始按鍵及滑鼠按鈕狀態，嘗試送出放開事件 |
| video.keyframe | Host | 要求下一個完整影格；合併一秒內的重複要求，重建影像編碼狀態 |

遠端顯示遇到等待關鍵影格或非關鍵影格解碼失敗時，若對端支援 `video.keyframe`，最多每兩秒提出一次請求。Host 在桌面靜止且持續擷取沒有新影格時，會嘗試一次一般截圖；擷取仍可能因 OS 權限或顯示狀態失敗，因此 accepted 不代表已收到完整影格。

遠端顯示正常關閉時會嘗試 `session.disconnect`，最多等待 500ms 後仍關閉本機 Peer。對端的成功回覆也可能在斷線過程遺失，因此連線已結束但未收到回覆時，不能宣稱已收到成功確認。

`input.reset` 供需要完整輸入重置的呼叫者使用。既有 `raw-key-reset` 仍保留同步送出順序，避免直接換成非同步 RPC 後，將下一次新按鍵誤放開。呼叫者使用完整重置時，應等回覆後才恢復輸入。

## 協定與擴充方式

`internal/p2p/commands.go` 提供 `RegisterCommand`、`RemoteCommands`、`SupportsCommand` 與 `CallCommand`，並以 `RegisterCommandParams`、`CallCommandParams` 支援最多 8 KiB JSON 參數。原有六項指令保持無參數介面，檔案與 Shell handler 以明確型別驗證並拒絕未知欄位。

控制訊息新增三種類型：

- `command-capabilities`：version 與實際支援的方法名稱；通道開啟及新 handler 註冊時公告。
- `command-request`：version、單一 Peer 內遞增 ID、method、expires、選用 params。
- `command-response`：對應 ID、結果或結構化錯誤代碼。

既有控制訊息保持原本路徑，沒有新增獨立事件種類。

指令版本為 1，預設呼叫逾時五秒，最多 32 個待回覆請求；接收端最多並行四項指令，回覆內容最多 16 KiB。每個 Peer 保留最近序號範圍內的結果，重複請求回傳已存結果或 in_progress；超出範圍的舊序號不重新執行。

常見錯誤為 unsupported_version、unsupported_method、expired_or_invalid、stale_request、busy、in_progress、failed、invalid_result。排入傳送佇列不等於執行成功；逾時或斷線會回報結果未確認。沒有自動重送副作用指令，呼叫者不能在結果未知時盲目換新 ID 重試。

expires 使用 Unix 毫秒，雙端系統時間需要合理同步；接收端不接受超過未來 30 秒的期限。handler 接收可取消 context，必須配合期限及 Peer 結束。新增功能僅在 handler 實際註冊後公告，能力清單不包含尚未實作項目。

舊版端點不公告此協定時，`SupportsCommand` 為 false，`CallCommand` 直接回報不支援。遠端顯示保留舊版影格恢復及本機關閉行為；不要求所有使用者同時更新。

## 遠端資料與 Shell

一般使用者 Host 新增 `files.roots`、`files.search`、`files.read`、`shell.run`，由 MCP 經遠端顯示子程序調用。搜尋與讀取限家目錄；Shell 使用 Host 帳號的 OS 權限，並不受家目錄讀取範圍限制。root Host 不註冊這四項能力。詳細限制見 [遠端資料與 Shell](MCP-REMOTE-DATA.md)。

## 後續可擴充項目

| 功能 | 需要先定義的條件 | 本次狀態 |
| --- | --- | --- |
| 畫面暫停／恢復、只看不控 | 本機拒絕權、在途輸入清理、恢復狀態 | 待實作，不公告 |
| 文字通知／聊天 | 顯示入口、長度上限、速率限制、歷史保存選擇 | 待實作，不公告 |
| 診斷快照、網路品質取樣 | 無敏感資料、時間範圍、取樣負載 | 現有本機診斷保留；跨端快照待實作 |
| 指定檔案傳輸、取消／續傳 | 使用者選定路徑、授權、完整性與續傳世代 | 現有剪貼簿傳輸保留；獨立傳檔擴充待實作 |
| 鎖定、登出、重啟、關機 | 明確授權與本機控制權、平台支援、結果交接 | 待實作，不隱含在通用指令內 |
| 升級、服務切換 | 套件驗證、管理員授權、回復方案 | 沿用本機流程，沒有遠端提權入口 |
| 關機後喚醒 | 裝置已離線，需要其他在線節點及硬體支援 | 無法由已關閉的 P2P 端點接收指令 |

本次只進行語法與編譯檢查，未進行額外實機互連測試；不應將尚未實作的擴充清單列入 Release 已支援功能。
