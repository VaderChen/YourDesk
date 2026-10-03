# 效能與記憶體最佳化（2026-10-03）

本輪減少影像、傳輸與聲音熱路徑的重複配置及複製，維持 UI、操作流程、功能、畫質、FPS、協定格式、佇列容量與壅塞控制。未變更 Android 程式或發行套件。

## 實作與所有權

- **JPEG 差分偵測**：重用 tile hashes／changed 暫存。前一畫面的 hashes 與工作區保持分離，所有區塊編碼成功後才交換；中途失敗仍可完整重試。RGBA 比對模式只把成功編碼的 patch 複製到基準，未變區域已逐位元組確認相同。單一 128×128 tile 更新只需複製 64 KiB，不再複製整張 RGBA。區域 view 直接使用 `SubImage` 回傳的獨立 descriptor，省去重複配置。
- **影格及 Tailcat 分片傳送**：同一次傳送重用一個分片暫存，不跨呼叫共用；保留每片標頭、時間戳、順序、限速及壅塞判斷。Pion SCTP 在 `Send` 返回前複製 payload；另以實際 localhost WebRTC 連線驗證後續分片與來源覆寫不影響收到的內容。Tailcat 單分片接收直接交付已複製、具有獨立所有權的資料，省去再次合併。
- **Core ML 待處理快照**：只重用從 jobs channel 取回、尚未被 worker 開始處理的 RGBA。大小、起點、stride 或像素長度不符就重新配置；worker 已取得的影格仍保持獨立。保留完整來源複製、最新影格替換及原本單張待處理上限。
- **聲音處理**：每個原生裝置重用 8 KiB 私有輸出暫存，回傳資料依實際大小獨立配置；沒有輸出的呼叫不再反覆配置 8 KiB。Opus 重用裝置私有的 20 ms PCM 暫存，輸出封包及解碼結果仍各自持有記憶體。裝置沿用固定 OS thread 的生命週期，原生暫存在 close 時釋放引用。

## 配置量測

環境：Apple M4、darwin/arm64、Go 1.27.1、`GOMAXPROCS=4`。使用相同 benchmark、修改前原始碼的 Go overlay 與修改後版本，各量測三次。下列為每次操作的 Go heap 配置量及配置次數；不是保留記憶體、原生／GPU 記憶體或整體程序 RSS。

| 情境 | 修改前 B/op | 修改後 B/op | 原／新 allocs/op |
| --- | ---: | ---: | ---: |
| 1080p 差分偵測，畫面未變 | 1,296 | 0 | 2／0 |
| 1080p 差分偵測，單 tile 更新 | 1,488 | 128 | 5／2 |
| 4K 差分偵測，畫面未變 | 4,608 | 0 | 2／0 |
| 4K 差分偵測，單 tile 更新 | 4,800 | 128 | 5／2 |
| Tailcat 傳送 60,000 bytes | 62,912 | 1,216 | 61／7 |
| Tailcat 單分片接收 | 5,256 | 4,104 | 15／14 |
| Core ML 替換待處理 1080p 快照 | 約 8,298,566 | 0 | 2／0 |
| Core ML 替換待處理 4K 快照 | 約 33,177,671 | 0 | 2／0 |
| PCM 編碼／解碼 | 8,192 | 4,096 | 1／1 |
| AAC 軟體編碼 | 8,192 | 約 321 | 1／1 |
| AAC 軟體解碼 | 8,192 | 4,096 | 1／1 |
| Opus 編碼 | 5,376 | 1,280 | 2／1 |
| Opus 解碼 | 8,192 | 4,096 | 2／1 |
| 原生聲音空輸出呼叫 | 8,192 | 0 | 1／0 |

差分 benchmark 使用不實際壓縮的測試編碼器，隔離偵測／基準更新成本；Compare 與 Hash 模式得到相同配置結果。Core ML benchmark 不執行模型，測量待處理快照管理；worker 已取得影格的路徑仍維持原本配置。原生聲音空輸出使用 PCM passthrough，無須開啟錄音或播放裝置。

實際 localhost WebRTC 1 MiB 影格的整條收送 benchmark，中位配置量約從 16.55 MB 降至 15.59 MB；包含 Pion、加密及接收端成本，因此不等於單一函式的配置量。影格傳送自身由每片配置改為每張一個最多約 28 KiB 的暫存。

測試期間系統另有大型編譯負載，耗時波動較大，不據此宣稱穩定的速度倍率、FPS 或端到端延遲改善。重用緩衝會保留少量工作記憶體，收益主要是降低配置流量、GC 壓力及多餘複製。

## 驗證

- 專案標準 Opus 設定的完整 `go test ./...` 通過。
- `internal/desktop`、`internal/p2p`、`internal/peertransport`、`internal/remoteaudio` 的完整 race 測試通過。
- Core ML 佇列測試通過，並以 race detector 重複 20 輪確認處理中影格不被改寫。
- 聲音 `GOEXPERIMENT=cgocheck2` 的輸出所有權、連續 Opus 及 AAC 往返測試通過；執行後續處理與 close 均不改寫已交付的結果。
- 差分測試涵蓋非零起點、額外 stride、尺寸重設、強制完整影格、第二塊編碼失敗後重試及像素回復。
- 傳輸測試涵蓋空資料、短末片、最大 datagram、逆序／重複分片、標頭／時間戳，以及接收或來源緩衝覆寫後的所有權。
- `gofmt` 與 `git diff --check` 通過。

主要重現命令：

```sh
# 僅為這次命令選擇可寫的快取，不更動全域 Go 設定。
export GOCACHE=/private/tmp/yourdesk-go-build-cache-20261003
export GOMAXPROCS=4

python3 scripts/opus.py darwin/arm64 go test -p 2 ./... -timeout=180s
go test -p 2 -race ./internal/desktop ./internal/p2p ./internal/peertransport -timeout=180s
python3 scripts/opus.py darwin/arm64 go test -p 2 -race ./internal/remoteaudio -timeout=120s
go test -p 2 -race ./cmd/remote -run '^TestMLQueue' -count=20
GOEXPERIMENT=cgocheck2 python3 scripts/opus.py darwin/arm64 go test -p 2 ./internal/remoteaudio -run '^(TestAudioOutputOwnership|TestOpusContinuous|TestNativeAACContinuous)$'

go test -p 2 ./internal/desktop -run '^$' -bench '^BenchmarkDeltaDetection$' -benchmem -count=3
go test -p 2 ./internal/peertransport -run '^$' -bench '^BenchmarkPacket' -benchmem -count=3
go test -p 2 ./internal/p2p -run '^$' -bench '^BenchmarkSendFrameWebRTC$' -benchmem -count=3
go test -p 2 ./cmd/remote -run '^$' -bench '^BenchmarkMLQueuedSnapshot$' -benchmem -count=3
python3 scripts/opus.py darwin/arm64 go test -p 2 ./internal/remoteaudio -run '^$' -bench '^(BenchmarkAudioProcess|BenchmarkNativeAudioEmpty)$' -benchmem -count=3
```

本機原始量測及驗證紀錄保存在 `.local-run/optimization-20261003/`，不加入版本控制。成功後移除本輪 `.bak`，聲音與差分偵測的修改前原始碼另保存在該目錄或 `.local-run/` 供 overlay 比較。

本輪未進行 Windows 實機、實際桌面長時間連線、Core ML 模型推論效能或系統錄放音驗收；沒有重建、簽章或發布安裝包。完整測試仍有既有 WebView／macOS API availability 與重複連結庫警告，沒有新增測試失敗。

以上為實作階段的量測及驗證記錄；後續跨平台套件、簽章與發布範圍見 [1.26.1003 build 1231 發行說明](RELEASE-1.26.1003-build-1231.md)。Windows 發布及更新使用 Portable ZIP，登入前服務另附同版服務 ZIP。
