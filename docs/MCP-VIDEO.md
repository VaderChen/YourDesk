# MCP 畫面契約 v1

本契約適用 `get_video_state`、`set_video_mode`、`snapshot`。回覆 `contractVersion: 1` 是 YourDesk 畫面契約版本，與 MCP 傳輸協議及 Signaling v1／v2 無關。原有工具及 `remote_action.screenshot` 保留；新增流程優先使用本契約。

## 建立連線

`connect` 的桌面選項 `videoMode` 可省略，或指定 `streaming`／`paused`，不可與 terminal 或 diagnostics 同時指定。省略維持串流；paused 完成協商後暫停。舊 Host 未提供此能力時保留連線、退回全螢幕串流，APP 顯示提示。協商前可能有少量在途影格。

connect 成功只表示啟動。等待 `get_status.processes[].stage == "connected"` 後使用畫面工具；暫停模式不需要先收到影格才會 connected。不要將隱藏 Viewer 視窗誤認為暫停串流。

## 工具與輸入

| 工具 | 必填 | 選填 | 行為 |
| --- | --- | --- | --- |
| `get_video_state` | `session` | 無 | 唯讀，不擷取、不切換模式、不依賴新影格 |
| `set_video_mode` | `session` | `mode`、`region`、`fullScreen` | 設定模式與共用視野 |
| `snapshot` | `session` | `mode`、`region`、`fullScreen` | 套用提供的設定，再取得 PNG |

session 使用 get_status 回傳值，例如 `viewer:站台ID` 或 `quick`，限桌面工作階段。

- mode：`paused`／`streaming`，省略保留現況。snapshot 不會自行恢復串流。
- region：`{x,y,width,height}`，四個欄位必填，皆為有限數值；x/y 非負、width/height 大於零，x+width 與 y+height 不得大於 1。
- fullScreen：true 清除範圍限制，不能與 region 同時指定。省略兩者保留現況。
- region 的基準是目前所選的**完整螢幕**，範圍由截圖與串流共用，不是相對上一張裁切圖片。

## 狀態回覆

get_video_state 的 JSON 位於 MCP structuredContent，主要欄位如下：

| 欄位 | 意義 |
| --- | --- |
| contractVersion | 固定 1 |
| mode、region、viewID | 最近確認的模式、範圍與視野版本；舊式全螢幕版本為 0 |
| display、displayCount | 選取螢幕索引（從 0 起）及數量；初始化尚未取得資訊時數量可為 0 |
| changing | 切換或結果尚未確認；true 時不要依最近確認的 mode/region 執行輸入 |
| inputReady | 畫面與視野已就緒；仍須同時確認 controlEnabled |
| controlEnabled | Viewer 是否允許鍵鼠控制；F12 停用時為 false |
| capabilitiesKnown | 是否已收到可辨識的 P2P 指令能力公告；false 不能直接斷言是舊版 |
| capabilities.region | 是否提供視野設定指令 |
| capabilities.pause | 是否提供暫停／恢復指令 |
| capabilities.freshSnapshot | 是否完整提供按需截圖及分塊讀取／關閉指令 |

查詢不改變模式。capabilitiesKnown=false 時可以稍後重查；不能只依 APP 版本或工具存在與否推定遠端支援。

## 設定與截圖回覆

set_video_mode 與 snapshot 的 JSON 位於 structuredContent，並以 JSON text content 提供同一份資料；snapshot 另外回傳 `image/png` image content。Agent 不需要解析自然語言訊息或讀取 Base64 分塊。

共同欄位：`contractVersion`、`session`、`mode`、`region`、`viewID`、`display`、`displayCount`、`legacy`。snapshot 額外提供 `width`、`height`、`fresh`；舊版可附帶 `notice`。寬高是實際 PNG 尺寸，最長邊 1280 像素。

- legacy=false、fresh=true：遠端 OS 重新擷取成功，不使用串流快取。fresh 表示本次重新擷取，不保證畫面此刻仍未改變。
- legacy=true、fresh=false：舊 Host 的最近收到影格，全螢幕串流仍持續，不是按需擷取。
- set_video_mode 沒有 fresh 欄位，因為沒有取得圖片。
- P2P token、分塊 bytes 與圖片二進位內容不放入 structuredContent。

## 座標契約

move／button 的 x/y 相對於**回傳圖片**：左上 `(0,0)`、右下 `(1,1)`、中心 `(0.5,0.5)`。Agent 不得自行再加 region 偏移。Host 負責區域偏移、Retina 像素與邏輯座標比例、多螢幕負原點換算；圖片縮小不改變相對座標。

範圍或螢幕切換後先重新 snapshot，再輸入。切換會釋放按住的鍵／按鈕；不能跨切換保留拖曳。Viewer 與 Host 使用 viewID 拒絕舊視野影格／座標，Agent 不需要自行傳入 viewID。

## 新舊相容與錯誤

| 情境 | 結果 |
| --- | --- |
| 新 Host、新 Viewer | 支援區域、暫停及按需截圖 |
| 舊 Host、全螢幕 snapshot | 回傳最近收到影格，明確標記 legacy/fresh |
| 舊 Host、區域或 paused 工具請求 | MCP isError=true；不修改範圍、座標或中斷連線 |
| 舊 Host、connect 指定 paused | 保留連線，降級全螢幕串流並提示；再查實際狀態 |
| 新 Host、舊 Viewer | 未啟用新指令時，維持原有影格標頭與全螢幕座標 |

參數錯誤、未連線、控制停用、逾時或不支援以 MCP 工具錯誤回報；錯誤文字不是穩定的機器判斷碼。不要從錯誤推定命令一定尚未生效。切換逾時後先查 changing，再重新設定／snapshot 確認，不能直接重送點擊。

snapshot 整體最多約 35 秒，單一 P2P 指令最多 5 秒；截圖最多 8 MiB，分塊 8192 bytes、最多三個並行讀取。普通模式設定約 10 秒逾時。以上均仍受呼叫端取消影響。

## 最小流程

1. connect：`{"id":"站台ID","videoMode":"paused"}`。
2. get_status 等待 connected，再 get_video_state：`{"session":"viewer:站台ID"}`。
3. 若 capabilities.region=true，snapshot：`{"session":"viewer:站台ID","region":{"x":0.25,"y":0.25,"width":0.5,"height":0.5}}`；舊版使用 fullScreen=true。
4. remote_action 點擊圖片中心：`{"session":"viewer:站台ID","action":"button","x":0.5,"y":0.5,"button":0,"down":true}`，隨後相同座標 down=false。
5. 需要串流時 set_video_mode：`{"session":"viewer:站台ID","mode":"streaming"}`；完成後 disconnect。
