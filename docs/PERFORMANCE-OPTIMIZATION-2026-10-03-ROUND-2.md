# 第二輪效能與記憶體最佳化（2026-10-03）

本輪接續既有的效能及檔案傳輸修改，維持 UI、操作流程、功能、影像像素、補幀排程、聲音格式及傳輸協定。正式程式只修改補幀快照、聲音擷取緩衝與剪貼簿 pull 傳輸／回收；未修改前端或發行套件。

## 修改與所有權

- **補幀待處理快照**：只重用 `frameInterpolator.queued` 尚未交付的 RGBA。交付時仍清空 queued，因此顯示中、前一張及原生推論中的影格都保持獨立。尺寸、起點、stride 或像素長度不相容時重新配置；停用、切換來源與方法仍走既有 reset。Apple 模式沿用原本工作尺寸與 ApproxBiLinear 縮放，逐位元組確認結果一致。
- **聲音 PCM 累積**：保留緩衝區起點，以讀取位置消耗完整影格，完成編碼及硬體備援後才搬移不足一包的尾端資料。擷取資料在複製前裁成原本的「最近六包」，每個啟用中的來源最多保留六包工作容量：PCM／AAC 為 24 KiB，Opus 為 22.5 KiB。停止、錯誤及設定世代切換會釋放引用。持續擷取不再因前移切片而反覆重新配置；封包內容、順序與備援仍相同。
- **剪貼簿 pull 分片**：同一讀取要求重用一個最多 16 KiB＋17 bytes 的封包緩衝。保留完整讀檔成功後才開始傳送的語意、每片標頭、短末片、流量控制及取消路徑。Pion SCTP 在 Send 返回前複製 payload；實際 localhost WebRTC 測試確認每次送出後立即覆寫來源仍能正確接收。
- **過期剪貼簿清單**：壓縮 cleanup 切片後清除底層尾端的舊參照，使過期清單與關閉函式捕捉的資源能被 GC 回收。保留原本十分鐘期限、目前清單與近期讀取清單的判斷及關閉次數。

聲音改用有界且可重用的工作容量，會保留少量暖機記憶體；收益主要是降低持續配置與 GC 壓力，不能單憑此推論整體程序的保留記憶體一定下降。

## 修改前後量測

環境：Apple M4、macOS 27.0.1、darwin/arm64、Go 1.27.1、`GOMAXPROCS=4`。以本輪開始前的來源建立 Go overlay，與修改後版本使用同一組 benchmark，各量測三次，以下採中位數。基準來源已包含前兩項任務的修改。

| 情境 | 修改前 B/op | 修改後 B/op | 原／新 allocs/op |
| --- | ---: | ---: | ---: |
| RIFE 待處理 1080p 快照替換 | 約 8,298,565 | 0 | 2／0 |
| RIFE 待處理 4K 快照替換 | 約 33,177,666 | 0 | 2／0 |
| Apple 待處理 1080p 快照替換 | 約 3,686,476 | 8 | 3／1 |
| Apple 待處理 4K 快照替換 | 約 3,686,472 | 8 | 3／1 |
| PCM 擷取，一次一包 | 8,960 | 4,864 | 2／1 |
| PCM 擷取，兩次半包組成一包 | 11,776 | 4,864 | 3／1 |
| Opus 擷取，一次一包 | 4,128 | 32 | 2／1 |
| Opus 擷取，兩次半包組成一包 | 6,944 | 32 | 3／1 |
| PCM 合成突波，100 包裁成最近六包 | 約 438,787 | 29,184 | 7／6 |
| Opus 合成突波，100 包裁成最近六包 | 約 385,219 | 192 | 7／6 |
| 剪貼簿 16 KiB 分片封裝 | 18,473 | 18,448 | 4／2 |
| 剪貼簿 256 KiB 分片封裝 | 295,332 | 18,448 | 49／2 |

這些數字是每次操作的 Go heap 配置量，並非 RSS、原生／GPU 記憶體或整體應用程式峰值。

- 快照 benchmark 測量複製／縮放及等待區替換，不執行 RIFE 模型或 Apple 原生推論。沒有可重用待處理影格時，配置量維持原本水準；Apple 殘餘的 8 bytes 來自既有工作尺寸查詢。
- 聲音 benchmark 使用同步模擬裝置，測量 Source 的累積、消耗及封包建立，未計入原生擷取／編碼的配置。frame／partial 每次操作交付一包；burst 是刻意放大的合成壓力情境，每次交付六包。32 bytes 是模擬 Opus 輸出的一包封裝，不代表真實 Opus 封包大小。
- 剪貼簿 benchmark 隔離分片封裝，不包含讀檔、網路、加密及接收端成本；256 KiB 情境的配置量減少約 94%。
- 量測期間耗時有波動，因此不據此宣稱 FPS、端到端延遲或固定速度倍率。

為使用同一 benchmark，baseline adapter 只替舊快照函式補上被忽略的重用參數，並將舊分片迴圈原樣抽成可提供 send callback 的函式；舊演算法與未調整的來源副本均保留在本機紀錄目錄。

## 驗證

- 專案 Makefile 使用的 Opus 設定完整 `go test ./...` 通過。
- 像素測試涵蓋非零起點、額外 stride、橫／直向、1080p、1×1 及緩衝尺寸不相容；所有快照與原縮放／複製結果逐位元組一致。
- 排程測試確認等待區替換不影響正在讀取的影格，交付後不再重用，停用後釋放所有影格參照。
- 音訊測試涵蓋空擷取、跨包殘餘、截斷、合成大突波、硬體回退及錯誤後換世代重新啟用，逐包比對 PCM 與封包順序。
- 剪貼簿測試涵蓋標頭、分片長度、來源覆寫、送出失敗、取消及過期清單參照釋放；實際 localhost WebRTC 收送驗證可重用來源緩衝。
- 修改前 overlay 確實在音訊容量上限及過期清單參照回收測試失敗；修改後兩者通過。
- `cmd/remote`、`internal/remoteaudio`、`internal/clipboard`、`internal/p2p` 完整 race 測試通過。
- 音訊與剪貼簿套件的 Windows amd64、CGO 關閉測試程式交叉編譯通過；不等同 Windows 實機驗證。
- Apple 原生補幀 smoke 通過：5 秒呈現收到 91 張真實影格及 49 張生成影格；雙視窗 20 秒橫直向切換及反覆停用／恢復產生 247 張生成影格，650 次停用檢查均通過。此為功能驗證，未據此宣稱速度改善。
- `gofmt` 與 `git diff --check` 通過。

重現主要檢查與量測：

```sh
export GOCACHE=/private/tmp/yourdesk-go-build-cache-20261003
export GOMAXPROCS=4
python3 scripts/opus.py darwin/arm64 go test -p 2 ./... -count=1 -timeout=180s
python3 scripts/opus.py darwin/arm64 go test -p 2 -race ./cmd/remote ./internal/remoteaudio ./internal/clipboard ./internal/p2p -count=1 -timeout=180s
YOURDESK_APPLE_SMOKE=1 python3 scripts/opus.py darwin/arm64 go test -p 2 ./cmd/remote -run '^TestApple(Presentation|Switching)Smoke$' -v -count=1 -timeout=90s
python3 scripts/opus.py darwin/arm64 go test -p 1 ./cmd/remote ./internal/remoteaudio ./internal/clipboard -run '^$' -bench '^(BenchmarkInterpolationSnapshot|BenchmarkAudioSourceCapture|BenchmarkPullChunkEncoding)$' -benchmem -benchtime=300ms -count=3
# 修改前版本：在 go test 加入 -overlay=.local-run/optimization-round2-20261003/baseline.json
```

本機原始紀錄與基準來源保存在 `.local-run/optimization-round2-20261003/`，不加入版本控制。本輪 `.bak` 於驗證成功後清除；既有的 `internal/video/codec.go.bak` 保留。

未驗證跨機長時間連線、Windows 實際執行、真實系統錄放音或 RIFE 模型效能。未重建、簽章或發布安裝包。既有 WebView／macOS API availability 與重複連結庫警告仍存在。

以上為實作階段的量測及驗證記錄；後續跨平台套件、簽章與發布範圍見 [1.26.1003 build 1231 發行說明](RELEASE-1.26.1003-build-1231.md)。Windows 發布及更新使用 Portable ZIP，登入前服務另附同版服務 ZIP。
