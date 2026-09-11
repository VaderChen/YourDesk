# 串流與影像增強參數設計

日期：2026-09-09。狀態：設計規格，尚未取代目前控制協定。

## 目標與目前預設

將來源擷取、編碼、遠端顯示 放大與補幀分開設定，再透過預設組合。禁止以「開啟影像增強」隱含改變其他參數，避免每次調整行為都需要修改遠端程式。

目前預設保持：增強關閉；開啟增強的預設組合只將來源寬、高各縮為 50%，選用 遠端顯示 放大演算法。碼率沿用原畫質模式，來源 FPS 不提高，補幀關閉。

新架構完成並更新雙方後，已支援參數的調整可透過連線協定套用。新的遠端編碼器、模型執行器或協定語意，仍需要對應端更新，無法僅靠預留參數實現。僅在 遠端顯示 執行的新放大模型則不應要求來源更新。

## 分層與責任

| 層級 | 責任 | 保存位置 |
| --- | --- | --- |
| 使用者偏好 | 預設組合、進階覆寫、啟用開關 | 遠端顯示 本機設定 |
| 來源要求 | 解析度、FPS、編碼與碼率要求 | 每個工作階段，傳送至來源 |
| 遠端顯示 處理 | 超解析度、模型、銳化、補幀、呈現限制 | 遠端顯示 本機，不傳模型選擇給來源 |
| 能力公告 | 各端實際支援的模式、範圍、限制及不可用原因 | 每次啟動／能力改變時探測 |
| 有效設定 | 協商後接受的完整值、偏離原因、實際編碼後端 | 工作階段狀態，不覆寫使用者偏好 |
| 遙測 | 實際尺寸、影片流量、FPS、耗時與丟幀 | 即時狀態，不當成設定 |

設定結構以型別化區塊組成，不繼續把功能旗標平鋪到 `p2p.Control`。目前 `Profile`、`ImageEnhancement` 等舊欄位僅留在相容轉接層。

## 參數目錄

表中的範圍是協定安全界線；實際可選範圍須與裝置能力交集。保留欄位不代表目前已有實作，未公告的欄位不得顯示為可用。

### 來源端 `source`

| 路徑 | 型別／選項 | 語意與界線 |
| --- | --- | --- |
| resolution.mode | native / scale / fit | 原始尺寸、固定比例、符合最大尺寸 |
| resolution.scalePercent | 整數 25～100 | 寬、高使用相同比例；scale 模式才適用 |
| resolution.maxWidth / maxHeight | 整數 1～8192 | fit 模式上限，保持比例，不放大來源 |
| resolution.viewportLimit | bool | 是否再受 遠端顯示 可用像素限制，明確控制，不隱含套用 |
| resolution.compute | auto / gpu / cpu | auto 優先 GPU；gpu 不可用時由 fallback 政策決定 |
| fps.mode | inherit / limit / multiplier | 沿用來源設定、固定上限、相對基準倍率 |
| fps.limit | 整數 1～240 | limit 模式的來源擷取／編碼上限，不是保證值 |
| fps.multiplierPercent | 整數 25～400 | 以未覆寫的基準 FPS 計算，再受能力上限限制 |
| codec.preference | 有序 codec ID 陣列 | 例如 hevc、h264、jpeg；取來源編碼與 遠端顯示 解碼的共同能力 |
| codec.backend | auto / hardware / software | 是選擇政策，實際後端另外回報 |
| rate.mode | inherit / quality / cbr / vbr | 沿用畫質基準、品質控制、固定／可變碼率 |
| rate.targetBps | 整數 64000～100000000 | cbr／vbr 的目標，每秒 bit，JSON 使用明確單位 |
| rate.maxBps | 整數，同上 | 僅支援峰值限制的 vbr 後端可用，必須 ≥ targetBps |
| rate.budgetPercent | 整數 25～200 | 僅基準具有明確 targetBps 時適用；不能對 quality 虛構碼率 |
| quality.value | 整數 1～100 | 邏輯品質值；各編碼後端映射不同，不能宣稱同數值畫質相同 |
| encoder.keyframeIntervalMs | 整數 250～10000 | 後端支援時才可選；不改變現有關鍵影格策略作為遷移副作用 |
| encoder.latency | interactive / balanced | 編碼延遲傾向；必須列出後端實際接受的選項 |
| color.range / chroma | auto / 明確格式 ID | 保留擴充點；需來源與解碼器共同公告才可設定 |

解析度比例以擷取原尺寸計算，永遠不以已縮圖尺寸再次折算。編碼對齊（例如偶數寬高）由來源解析器處理，分別回報有效內容尺寸及編碼尺寸，不能造成游標座標偏移。

`inherit` 表示維持來源目前基準政策，不代表固定數字零。`quality` 沒有精確碼率承諾；選擇 budgetPercent 時若基準沒有碼率，必須拒絕該組合或要求使用者改為明確 targetBps，不能自行估一個低碼率。

### 遠端顯示 端 `viewer`

| 路徑 | 型別／選項 | 語意與界線 |
| --- | --- | --- |
| upscale.enabled | bool | 只控制本機放大，不隱含改變來源 |
| upscale.backend | auto / fsr1 / coreml / 後端 ID | 可新增 ID，依能力目錄顯示名稱與說明 |
| upscale.model | 字串 ID | 例如 pipersr-2x；是已安裝且驗證的模型 ID，不接受遠端路徑或 URL |
| upscale.compute | auto / gpu / neural / cpu | 視後端能力而定；Core ML 自動排程不可標示成已保證 ANE |
| upscale.sharpness | 整數 0～100 | 統一 UI 尺度，由後端映射；不支援時停用 |
| upscale.maxProcessingMs | 整數 1～2000 | 處理預算；後端須公告是否能中斷單次運算 |
| upscale.queueDepth | 整數，第一版固定 1 | 只保留最新待處理影格，不累積延遲 |
| interpolation.mode | off / optical-flow / 後端 ID | 只有實作完成才公告，第一版僅 off |
| interpolation.multiplier | 整數 2 / 3 | 每對真實影格補 1／2 張，與來源 FPS 完全獨立 |
| interpolation.maxAddedLatencyMs | 整數 0～200 | 延遲限制；無法滿足時回退，不以重複影格冒充補幀 |
| presentation.maxFPS | 整數 1～240 | 顯示上限，再限制於螢幕及後端能力 |
| presentation.sync | auto / vsync | 依呈現後端支援，不任意切換原生全螢幕 |
| fallback.backends | 有序後端 ID 陣列 | 例如 coreml → fsr1 → linear；必須無循環且有可用終點 |

模型能力須帶輸入／輸出格式、倍率、固定或彈性尺寸、分塊需求及支援運算裝置。PiperSR、QuickSRNet、SESR、ESPCN、Real-ESRGAN 等候選沿用同一份模型描述，不各自新增主協定旗標。

## 組合規則

解析順序固定為：來源預設 → 畫質預設 → 增強組合 → 使用者明確覆寫 → 雙方能力限制。每次從原始基準重新解析，禁止對上次有效設定再乘比例。

| 組合 | 解析度 | 碼率／品質 | 來源 FPS | 遠端顯示 |
| --- | --- | --- | --- | --- |
| 原畫質 | 沿用畫質模式 | 沿用畫質模式 | 沿用畫質模式 | 增強關閉 |
| 增強・保留品質（目前） | 原寬高 50% | 沿用畫質模式 | 沿用畫質模式 | 選定超解析度，補幀關閉 |
| 增強・節省頻寬（未開放） | 原寬高 50% | 使用者指定碼率 | 沿用畫質模式 | 選定超解析度 |
| 增強・高更新率（未開放） | 原寬高 50% | 沿用畫質模式 | 基準 2×，受能力限制 | 選定超解析度 |
| 自訂 | 個別覆寫 | 個別覆寫 | 個別覆寫 | 個別覆寫 |

主開關控制「增強組合」是否參與解析；停用時保留偏好，但移除該組合施加的覆寫。畫質預設仍可獨立切換。使用者若修改預設組合中的任一值，介面標示「自訂」，可一鍵還原該組合。

## 版本化協定

新增單一 `stream-config` 訊息入口，內含 `schemaVersion`、工作階段 ID、遞增 `revision` 與完整 source 快照。採完整快照而非增量 patch，避免遺漏舊值、無法區分省略與刪除。完整快照中未提供的可選欄位使用明確的 schema 預設；false 與 0 不得等同於未指定。

示意要求（尚非目前線上協定）：

```json
{
  "type": "stream-config",
  "schemaVersion": 1,
  "sessionId": "negotiated-session-id",
  "revision": 7,
  "capabilityRevision": 2,
  "source": {
    "resolution": {"mode": "scale", "scalePercent": 50, "viewportLimit": false, "compute": "auto"},
    "fps": {"mode": "inherit"},
    "codec": {"preference": ["hevc", "h264", "jpeg"], "backend": "auto"},
    "rate": {"mode": "inherit"}
  },
  "requiredPaths": ["source.resolution.mode", "source.resolution.scalePercent"]
}
```

遠端顯示 的模型與呈現設定不包含在來源要求中。來源公告 `stream-capabilities`，包括 schema 版本、capabilityRevision、支援欄位路徑、enum／min／max／step，以及跨欄位限制，例如某編碼後端僅支援 quality、不支援 vbr。

回覆 `stream-config-result`，包含相同 revision、accepted／rejected、完整有效設定、各欄位偏離原因。未知必填欄位或模式拒絕整份要求；未知選填欄位明確列為 unsupported，保留舊有效設定中不受影響的能力。不得靜默回報成功。

範例：要求 120 FPS，但能力上限 60，回報 effective.fps = 60 與 reason = clamped-to-capability。要求必填的 50% 縮圖但來源不支援，整份要求拒絕，原串流繼續。

## 套用與失敗處理

1. 接收執行緒只驗證大小、版本、工作階段、revision 及型別，再交給影像工作執行緒。限制訊息大小及清單長度，不在控制執行緒載入模型或重建編碼器。
2. 解析、範圍檢查及跨欄位驗證完成後，準備必要資源。失敗保持上一組有效設定，回覆明確原因。
3. 在影格邊界原子套用。只有尺寸、編碼格式或後端要求時才重建編碼器；單純碼率或 FPS 調整優先使用後端動態設定。
4. 發出 applied 回覆後，影格中帶 `configRevision`。新舊影格混在網路中時，遠端顯示 依 revision 判斷，不將舊畫面配上新的綠燈狀態。
5. 調整尺寸採 debounce，佇列最多保留最新要求。同 revision 重送直接回傳已知結果；較舊 revision 忽略，不倒退套用。
6. 超時可重送相同 revision，不自行遞增形成重建風暴。重連建立新 sessionId，重新公告能力並套用偏好。
7. 顯示器切換、解碼尺寸改變及本機後端切換時清除對應的模型／補幀歷史；實際結果只供狀態顯示，不寫回使用者偏好。

需要修改影格標頭時使用已協商的新格式；未協商前維持目前格式。不能只在舊 frame header 直接插入欄位。

## 介面與持久化

「影像增強」頁保留主開關、預設組合與超解析度分組下拉。另提供進階區，分為來源解析度、編碼品質、來源 FPS、本機放大、補幀／呈現。只顯示已實作且可用的控制；尚未支援的選項顯示原因，不能僅因 schema 預留就開放選取。

可用性要區分「本機不可用」、「遠端未支援」、「正在偵測」、「目前組合不相容」。調整前顯示預估有效參數，套用後顯示實際值；遠端不存在時仍可儲存偏好，但不能顯示已生效。

持久化另設 `settingsVersion`，與網路 schemaVersion 分開。保存預設組合 ID、presetVersion 及明確覆寫；舊設定遷移一次，未知未來欄位保留在受限的擴充區，不能因較舊程式儲存設定而全數刪除。所有顯示文字用翻譯鍵，協定只使用穩定 ID。

## 遙測與圖示

統一回報：requestedRevision、appliedRevision、來源／內容／編碼／顯示尺寸、codec 與實際編碼／解碼後端、目標碼率、影片實際位元組、來源／解碼／生成／呈現 FPS、處理耗時、丟幀數、目前 fallback 與原因。

灰色表示停用、等待或沒有實際增強輸出；亮綠色表示當前影格使用有效的增強結果。Hover 不能只顯示使用者選了哪個模型；必須顯示實際後端。品質控制模式顯示「品質控制」，不能將 0 bps 顯示為實際流量零。

## 遷移順序

1. 建立獨立 `internal/streamconfig` 型別、驗證器、預設組合解析器及能力交集；預設輸出須等同目前行為。
2. 新增統一協定、revision 回覆與舊協定 adapter。新對新使用新協定；新 遠端顯示 對舊來源，只轉換能忠實表示的設定，其他標示不支援；舊 遠端顯示 對新來源保留既有行為。
3. 將來源 ticker、縮圖器、encoder 參數集中從有效設定取得，移除與 ImageEnhancement 旗標綁定的策略分支。
4. 遠端顯示 的 FSR／Core ML 與未來模型共用 backend 能力描述，設定頁由同一份描述產生選項及限制。
5. 最後開放進階調整。預留但尚未完成的色彩與補幀能力繼續停用，不在此次設計階段改動現有串流。

此文件完成設計，尚未實作以上遷移，沒有修改目前 FPS、碼率或解析度行為。


## 舊參數相容契約（必要條件）

舊參數與舊訊息格式不得刪除、改名或改用新型別。新結構由 adapter 對接，不能直接要求所有使用者同步升級。

| 既有參數／訊息 | 相容方式 |
| --- | --- |
| quality-profile 與 Profile：fast／standard／high | 保留目前畫質預設語意，轉為對應基準組合 |
| ViewWidth／ViewHeight | 保留目前合法範圍及原本低流量視窗限制；不是新增所有模式都受視窗限制 |
| ImageEnhancement | 保留目前寬高各半、碼率沿用、FPS 不變的策略，轉為增強組合 |
| EnhancementSupported／EnhancementReport | 繼續公告並服務既有 遠端顯示；不當作支援新 schema 的證據 |
| preferences.json 的 imageEnhancement／superResolution | 保留讀取與寫入相容欄位；僅能表達舊語意時才寫入等價投影 |
| 目前使用的 FPS、Quality、Codec 啟動參數 | 繼續作為來源基準輸入；新版設定的 inherit 以這些值解析 |
| 既有影格標頭與 codec ID | 未完成新格式協商時照舊傳送，不能直接改長度或欄位位置 |

連線矩陣：

- 新 遠端顯示／新來源：先明確協商 stream-config schema；成功後同一工作階段只使用新設定入口，停止傳送會覆蓋新設定的 quality-profile 心跳。
- 新 遠端顯示／舊來源：維持 quality-profile；只有可忠實映射的設定才送出。任意碼率、額外 FPS 倍率等不能表示的覆寫保持在本機偏好中，介面提示需要更新遠端，不回報已套用。
- 舊 遠端顯示／新來源：持續接受舊參數，經 adapter 產生完整內部設定；不要求 revision，不改舊 遠端顯示 的預設行為。
- 舊 遠端顯示／舊來源：完全不受此次新協定影響。

新協定拒絕某個設定時，不得暗中再發送舊參數嘗試達成近似效果。協商完成後收到過時的舊設定訊息，不可覆蓋已套用的新 revision。只有重新建立工作階段或明確切回 legacy 模式，才重新接受 legacy 設定控制。

設定檔若同時存在新舊欄位：有效且受支援的 settingsVersion 使用新結構；尚未遷移的檔案由舊欄位建立一次等價設定。降版無法表達的進階覆寫不可塞進舊參數，另保留新設定資料供回升版本恢復。避免兩份欄位各自更新而互相覆蓋，儲存必須由同一個序列化器完成。

相容驗收規劃應包含上述連線矩陣、舊設定檔載入、未知 schema、重送及切換畫質，不只驗證新版與新版。本文只定義要求，本次未額外執行測試。


## 本次接入範圍

第一版程式已新增 `internal/streamconfig`，集中預設組合、驗證、尺寸／FPS／品質／碼率解析。增強開關透過 `Compose` 產生 resolution.scalePercent=50、fps.mode=inherit、rate.mode=inherit，模型仍留在 遠端顯示。

`Control` 新增 streamConfig、streamCapabilities、streamResult 包裝，舊欄位完整保留。來源每個工作階段產生獨立 sessionId；新版 遠端顯示 收到 schemaVersion=1 公告後改用新入口、遞增 revision 並以同 revision 重送。來源忽略舊 revision，重送既有結果；接管新入口後忽略 legacy 畫質訊息。舊端透過 Compose 轉接至相同解析器。

目前可解析 resolution 的 inherit/native/scale/fit、FPS 的 inherit/limit/multiplier、rate 的 inherit/quality/cbr，以及品質值（0 表示沿用預設）。尚未開放進階 UI，僅現有增強組合使用新入口。碼率後端失效時有效結果回報品質控制及原因，不保證 CBR 精確達標。

結果回報有效尺寸、FPS 上限、碼率與品質；EnhancementReport 增加 configRevision，圖示等待對應設定回報後才更新狀態。既有影格標頭未改，因此尚非逐影格 revision 標記；極短暫的在途舊影格仍可能存在。原設計中的新影格格式、通用 requiredPaths、完整能力範圍描述、設定檔版本遷移與其他預留參數尚未在本次實作，不宣稱已支援。

本次保持原有解析度、碼率及 FPS 預設。新協定需要雙方新版；不同版本仍可使用原有品質與增強設定。


## 策略選單接入

影像增強頁新增「保留畫質／流暢優先／流量優先」，預設保留畫質，並持久化到 enhancementStrategy。流暢與流量策略共用可調的 enhancementBitrateMbps（1～100，預設 12 Mbps）；這是使用者指定預算，不是自動量測的原始流量。

保留畫質沿用 rate=inherit、fps=inherit；流暢使用 rate=cbr、完整預算、fps.multiplierPercent=200；流量使用 rate=cbr、預算一半、fps=inherit。增強關閉時不套用策略覆寫。全部透過既有 schema v1 參數組合，不新增來源控制欄位；已支援該協定的來源不必再為此次選單更新。舊來源或能力不足維持原畫質策略，Hover 提示限制。實際 FPS 與碼率仍受編碼器、裝置及網路影響。


## 增強解析度調整

新版增強組合改為原始寬、高各 75%（1920×1080 → 1440×810），碼率及 FPS 仍由策略決定。沿用 schema v1 的 scalePercent，已支援新協定的來源不需要更新。舊旗標由 ComposeLegacy 保留 50% 語意；新版 遠端顯示 對舊來源會提示其半解析度限制。
