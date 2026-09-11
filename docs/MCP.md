# Agent 操作介面（MCP）

MCP 預設關閉。請至「設定 → MCP 設定」開啟「啟用 MCP」，即可讓支援 Streamable HTTP 的 Agent 查詢狀態、建立遠端連線及操作遠端桌面。開關會自動儲存，切換後立即啟動或停止服務；重啟 APP 會恢復上次選擇。服務綁定 `0.0.0.0:12345`，路徑為 `/mcp`，隨 APP 完整結束而停止。

## 連接方式

設定頁顯示 `http://127.0.0.1:12345/mcp`，旁邊的複製圖示可複製此本機連線位址。實際監聽為 `0.0.0.0:12345`；其他電腦使用這台電腦的實際 IP，並依下列白名單／Token 規則驗證。

IP 白名單預設開啟，初始只有 `127.0.0.1`。在「MCP 設定」可切換白名單或編輯允許的 IP（每行一個，最多 128 個），儲存後立即套用到後續請求。未列入白名單的來源會收到 HTTP 403；啟用時不允許空白名單，格式錯誤不會儲存。白名單檢查實際 TCP 對端 IP，不信任 `X-Forwarded-For` 等轉送標頭；使用代理時需將代理來源位址納入白名單，並由代理限制原始來源。

**白名單啟用時：名單內免 Token，名單外回傳 HTTP 403，即使帶有 Token 也不放行。白名單關閉時：所有來源均須有效 Bearer Token，缺少或不符回傳 HTTP 401。**

需要 Token 時，請帶入 `Authorization: Bearer <Token>`。首次啟用會產生獨立 Token，不使用遠端密碼：

- Windows：`%APPDATA%\YourDesk\mcp-token`
- macOS：`~/Library/Application Support/YourDesk/mcp-token`

在 Agent 的 MCP 設定中填入 URL 與 Authorization 標頭即可。Token 可由 `YOURDESK_MCP_TOKEN` 環境變數提供（至少 32 字元）；`YOURDESK_MCP_DISABLE=1` 可停用服務。Token 不出現在 MCP 回應或 LOG；修改 Token 後須完整重啟 APP。

這個 HTTP 入口不接受瀏覽器 Origin 請求，也不會自動修改 Windows 防火牆。跨機使用時請限於可信任網路，或透過 VPN／HTTPS 代理保護 Token 與資料。12345 已被佔用時，APP 仍可執行，並記錄 MCP 啟動失敗。

## 可用功能

| 工具 | 用途 |
| --- | --- |
| `get_status` | 本機版本、裝置 ID、管理中的程序 PID／父 PID、角色、連線狀態及 Host 佔用提示 |
| `list_sites` | 列出已儲存站台，不包含密碼 |
| `connect` | 以站台 `id` 或裝置 `room` 連線，仍需正常密碼驗證；`secret` 可省略以使用已記住的密碼 |
| `disconnect` | 中斷指定遠端顯示 `session`，不能停止 Host |
| `get_preferences` | 讀取 APP 設定 |
| `set_preferences` | 修改提供的設定欄位，其餘保留；修改影像傳輸或 IP 直連設定會重新啟動 Host |
| `connection_diagnostics` | 讀取已連線站台的串流取樣；`start=true` 開始新一輪取樣 |
| `remote_action` | 取得遠端畫面、移動／按下／放開滑鼠、鍵盤輸入、捲動、貼上文字及切換螢幕 |


## 背景操作與提示

「MCP 接管後開啟畫面」預設關閉，說明以泡泡提示呈現。透過 MCP 新建的遠端連線預設不開啟顯示視窗，Agent 仍可取得畫面與操作；開啟設定後，下次接管會自動顯示畫面。MCP 連線存在時，Tray 圖示變為橘色，並顯示「MCP 正在操作遠端」泡泡提示；連線全部結束後恢復原圖示。

Tray 選單會出現「開啟 MCP 遠端畫面」，開啟後切換為「隱藏 MCP 遠端畫面」，可反覆切換，不必中斷連線。若有多個 MCP 連線，選單一次切換這些畫面。首次接管原本手動開啟的連線時，也依此設定決定是否顯示；接管後調整設定不會打斷當前畫面。

## 遠端操作順序

1. 呼叫 `list_sites`，再以 `connect` 建立連線。回應成功代表「已啟動」，不代表驗證已完成。
2. 呼叫 `get_status`，等待目標 `session` 的 `stage` 為 `connected`。
3. 呼叫 `remote_action`，例如 `{"session":"viewer:站台ID","action":"screenshot"}`。回傳 PNG 最長邊為 1280 像素，內容是最新收到的解碼畫面。
4. 滑鼠使用畫面相對座標（0～1）：`move` 移動；`button` 的 `button=0/1/2` 代表左／右／中鍵，`down=true/false` 代表按下／放開。按下後必須放開。
5. 鍵盤用 `key`、`key="a"` 等名稱與 `down`，組合鍵依序按下，再反向放開。文字用 `text` 與 `text` 欄位，最多 16 KB；此操作會取代本機與遠端剪貼簿文字並貼上。
6. 捲動用 `scroll`、`delta`（-100～100）；切換螢幕用 `display`、`display` 索引（從 0 開始），切換後重新取得畫面。
7. 完成後用 `disconnect` 結束遠端顯示。

操作逾時或取消後，請先重新取得畫面確認結果，不要直接重送點擊或文字。30 秒未有輸入操作會釋放 Agent 留下的按鍵／滑鼠；遠端顯示內用 F12 停用控制，也會拒絕後續 Agent 輸入。背景分析連線不接受畫面操作。

`connect` 使用已儲存站台並指定 `diagnostics=true`，可建立不顯示視窗的背景分析連線。MCP 不會繞過連線密碼或作業系統的螢幕擷取／輔助使用權限。

實作採用官方 [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk)，使用 Streamable HTTP 與獨立 Bearer 驗證。
