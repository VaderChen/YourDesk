# 函式級效能與記憶體最佳化（2026-10-03）

本輪以全專案盤點、函式基準測試與行為回歸確認改善，維持 UI、操作流程、JSON／影音協定、權限及既有功能。保留同一工作目錄中尚待發行的 Android APK 自動更新功能。

## 盤點範圍

修改前以 Go AST 索引 **1,564 個自有具名函式、391 份含函式的 Go 原始檔**，包含不同平台的 build-tag 檔案：`cmd` 267 個、`internal` 1,149 個、獨立的 `android/core` 模組 148 個。這是靜態盤點數量，不表示每個函式都需要改寫，也不是端到端效能分析。

依迴圈、配置、複製、JSON／字串處理及呼叫頻率檢視影音、傳輸、檔案、終端機、站台管理、服務與更新路徑，另檢視 Android Java、前端 JavaScript、原生平台與建置入口。第三方套件、模型、預建二進位及自動產生檔案不納入改寫。

前兩輪已改善的畫面差分、P2P 分片、音訊、剪貼簿與影像緩衝重用維持既有實作；授權、TLS、命令排序、原生硬體選擇及發行腳本亦保留。詳見 [第一輪](PERFORMANCE-OPTIMIZATION-2026-10-03.md) 與 [第二輪](PERFORMANCE-OPTIMIZATION-2026-10-03-ROUND-2.md)。

## 實際修改

| 範圍 | 函式與改善 | 保留的條件 |
| --- | --- | --- |
| AV1 | `walkAV1OBUs`、`av1Keyframe`、`packAV1` 直接巡訪 OBU，避免建立整份描述清單；補入 sequence header 時只配置一份輸出 | 32 MiB、4,096 OBU 上限；原有錯誤優先順序；完整語法驗證後才改變 sequence 快取；輸出不覆寫快取 |
| Android 影格解析 | `EncodedVideoFrame.parse`／`annexB` 先驗證 NAL，再一次複製 AU 並替換長度標頭；CSD 直接配置最終大小 | AVC／HEVC 格式、參數順序、IDR／IRAP 判定；輸入、CSD、AU 各自擁有資料；無效封包不配置大型 AU |
| Android Go 影格佇列 | `enqueueSessionFrame`、`popFrame`、`resetFramesLocked` 改用逐步擴充的環狀佇列，取出不再搬移剩餘指標 | FIFO、512 筆／32 MiB 限制、世代隔離、溢位後等待 keyframe；取出清除引用，重連／關閉釋放佇列 |
| 終端機 | `read`、`appendOutput`、`readOutput` 重用最多 256 KiB 的工作緩衝；固定欄位回應改用結構 | 原有 JSON 欄位與空值、4 KiB 回應、ACK 重送、關閉喚醒；每份待傳回應仍獨立複製，避免序列化時被覆寫 |
| 檔案內容搜尋 | `search`／`readSearchContent` 在一次搜尋內重用緩衝，依檔案大小提示配置並有界擴充 | 每檔最多 64 KiB、UTF-8／NUL 檢查、符號連結限制、逾時與結果上限；檔案增長仍可讀到上限，短檔不搜尋前一檔殘留資料 |
| 桌面前端 | `refreshSiteDeviceStatuses`、`refreshSiteCapabilities`、`renderLibrary`、`renderSites` 使用當次更新的索引與群組計數；空白查詢略過字串組合；`queueSummary` 一次累計 | 相同 DOM、排序、群組、查詢、能力按鈕與進度值；重複 ID 仍取第一筆；索引不跨更新快取，避免陳舊站台資料 |

日期格式器重用在本機量測未得到穩定收益，`renderList` 的日期格式維持原實作；日期顯示同時納入四語系回歸檢查。

## 函式量測

環境：Apple M4／macOS arm64、Go 1.27.1（`GOMAXPROCS=4`）、OpenJDK 17.0.20.1、Node.js 24.14.0。每項取三次結果的中位數；前後使用相同輸入及測試程式。配置量是每次呼叫／操作累計配置的 bytes，**不是整個 App 的常駐記憶體**。

| 測試情境 | 修改前 → 修改後配置量 | 修改前 → 修改後耗時 |
| --- | ---: | ---: |
| AV1 一般影格驗證 | 400 B／3 次 → **0 B／0 次** | 73.64 → 14.50 ns |
| AV1 4,096 OBU 驗證 | 761,682 B／15 次 → **0 B／0 次** | 71.24 → 16.21 µs |
| AV1 64 KiB delta 打包 | 73,976 B／5 次 → 73,728 B／1 次 | 3.172 → 2.955 µs |
| 終端機每次 4 KiB、無積壓 | 8,560 → 4,160 B | 958.0 → 463.5 ns |
| 終端機每次 4 KiB、64 KiB 積壓 | 19,482 → 4,160 B | 1,678.0 → 516.9 ns |
| 搜尋 100 個 128 B 檔案、無匹配 | 143,472 → 90,384 B | 1.199 → 1.201 ms（大致持平） |
| 搜尋 100 個 64 KiB 檔案、無匹配 | 13,903,566 → 155,409 B | 3.016 → 1.629 ms |
| Android Go 512 筆佇列整批進出 | 40,960 B，維持原值 | 18.961 → 9.812 µs |
| Java AVC 1 MiB picture 解析 | 2,097,424 → 1,048,760 B | 64.654 → 31.878 µs |
| Java HEVC 1 MiB picture 解析 | 2,097,536 → 1,048,776 B | 69.854 → 27.552 µs |

Android 佇列仍為每個有效影格保留獨立描述物件；改善的是取出時的搬移成本。Java 測試另外涵蓋 256 KiB 與 4 MiB picture，配置量同樣約減半，並未移除 MediaCodec 所需的獨立 AU／CSD。

前端以實際函式配合簡化 DOM 測試殼量測：5,000 筆站台狀態更新由約 77.2 ms 降至約 0.4 ms；2,000 筆傳輸進度彙總由約 0.038 ms 降至約 0.014 ms。此數據不包含瀏覽器 layout／paint，也不代表少量站台或實際遠端連線會獲得相同比例提升。

## 驗證與重現

新增及既有測試涵蓋：AV1 格式與錯誤順序、真實 GOP fixture、sequence 快取所有權；搜尋短檔／增長檔／64 KiB 邊界／取消／符號連結；終端機 ACK、回應所有權、20,000 次交錯讀寫及滿緩衝關閉；Android 佇列環繞、增長、引用回收、10 萬影格壓力與並行關閉；Java 畸形 NAL、4,096 筆界限、輸入／CSD／AU 隔離、10,000 次隨機封包及大型 AU 壓力。

- 全部根 Go 模組測試通過；影音、終端機、搜尋及 Android core 的 race 測試通過。
- 真實本機 WebRTC／P2P／PTY 終端機 smoke 通過，包含輸入去重、關閉與突然斷線回收。
- Java host 測試通過 **10,620** 個斷言；Android policy 測試通過。
- 真實無頭瀏覽器桌面 smoke 通過，新增 500 站台／40 群組、篩選、能力狀態更新檢查；保留四語系、上傳、取消 ACK 遺失、重連及關閉測試。檔案傳輸 JavaScript 測試 **57** 項通過；Android 前端 **26** 項回歸通過。
- Android core AAR 重建及來源指紋、個人路徑、16 KiB ELF 檢查通過；Gradle `testDebugUnitTest`、`lintRelease`、`assembleDebug`、`assembleRelease` 通過，**25** 項 JUnit 無失敗。Lint 為 0 error、33 warning，與修改前數量相同。
- Windows amd64 的 `video`、`terminal`、`remotedata`、`clientui` 測試程式交叉編譯通過（`CGO_ENABLED=0`）；不等同 Windows 原生硬體執行測試。

上述函式測試使用本機 Debug 與未簽章候選產物；後續 `1.26.1003 build 2028` 已以原正式金鑰完成 APK 簽章及 ZIP 封裝。Android 實機解碼、長時間跨機連線及實機覆蓋升級尚未在本輪執行。

函式基準測試入口：

```sh
python3 scripts/opus.py darwin/arm64 go test ./internal/video ./internal/terminal ./internal/remotedata \
  -run='^$' -bench='Benchmark(AV1Keyframe|PackAV1Delta|TerminalOutput|SearchContent)$' -benchmem -count=3
(cd android/core && go test -run='^$' -bench=BenchmarkFrameQueue -benchmem -count=3)
node scripts/benchmark-functions-ui.cjs --bench
VIDEO_TEST_JDK="$JAVA_HOME" bash android/app/src/testHost/run-video-tests.sh
```

Java 配置量由 `VideoFrameBenchmark` 使用 JDK `ThreadMXBean` 量測，直接編譯該類別與 `EncodedVideoFrame` 後，以 `java -Xms128m -Xmx128m` 執行。這是 JVM parser 測試，並非 Android 實機硬體解碼測試。

本機證據存放於 `.local-run/function-optimization-20261003/`，包含函式索引、修改前來源、備份雜湊、原始 benchmark 輸出、`benchmark-medians.json`、測試日誌與瀏覽器截圖。前後 Go 比較使用 overlay：終端機只將舊 handler 內的原始 read／append 內容抽成測試入口；搜尋的新增 helper 僅以不會被舊 benchmark 呼叫的 stub 協助編譯；Android 舊佇列使用修改前私有欄位測試與相同 benchmark 主體。沒有以新演算法替代舊基線。

修改前建立本輪 `.bak`，驗證完成才移除；既有 `internal/video/codec.go.bak` 保留。Windows 發行與更新仍採 **Portable ZIP／Service ZIP**，最佳化階段未更改版本或發行格式。後續桌面發行整理至 [1.26.1003 build 2028](RELEASE-1.26.1003-build-2028.md)；本次包含正式簽章 Android ZIP，既有版本首次須手動安裝，之後才能使用新增的自動更新器。
