# 記憶體最佳化：第一輪

基於 `4957f66`，本輪不修改協定格式、不擴大解碼佇列、不變更 GOP 恢復策略。

## 已實作

- macOS 持續擷取先取得 retained pixel buffer，確認有新影格後才配置 Go RGBA。沒有更新或等待逾時時，不再配置整張像素。take/copy/release 分離，擷取關閉不會使已取得的 buffer 失效。
- macOS H.264／HEVC 原生解碼直接把 retained pixel buffer 轉換到獨立 Go RGBA，取消中間的 C RGBA malloc 及 `C.GoBytes` 全幀複製。每張輸出仍具有獨立所有權，不會因下一幀或 decoder 關閉而被覆寫。
- P2P 單分片只複製一次；多分片改用可重用的索引切片，按實際收到的總長度配置輸出。重複分片不配置，完成後不重複交付；完成／新序號出現時清除舊分片引用。拒收超過發送端 `chunkSize` 的分片。
- 多分片仍保留分片複製與最終合併複製，未改成直接引用網路 buffer，也不依第一個未驗證完整的影格宣告預先配置數十 MiB。

## 基準

本機 Apple M4、darwin/arm64；相同 benchmark，舊實作用 Go overlay 指向修改前備份，不切換或覆蓋工作樹。命令：

```sh
go test ./internal/p2p -run '^$' -bench '^BenchmarkFrameAssembler$' -benchmem -benchtime=500ms -count=3
```

下列為三次量測的中位數，配置量為每次重組，不是 RSS：

| 壓縮影格大小 | 舊 B/op | 新 B/op | 舊／新 allocs/op | 舊／新 ns/op |
| --- | ---: | ---: | ---: | ---: |
| 4 KiB | 33,104 | 4,096 | 4 / 1 | 4,929 / 915.3 |
| 256 KiB | 549,531 | 524,533 | 15 / 12 | 98,134 / 92,051 |
| 1 MiB | 2,115,934 | 2,098,055 | 42 / 39 | 298,372 / 277,845 |

4 KiB 配置量下降約 87.6%；這次微基準約快 5.4 倍。大型影格時間波動大，不宣稱穩定的速度增益。這些數字不是實機 FPS 或端到端延遲。

## 驗證

- `go test ./...`：通過；仍有第三方原生 API 棄用及重複連結庫警告。
- `go test -race ./internal/p2p ./internal/streampipeline ./internal/video ./internal/desktop -count=1 -timeout=180s`：通過。
- 分片測試涵蓋可變長度、空分片、逆序、重複、舊序號、未完成影格取代、輸入覆寫後輸出仍獨立。
- Worst-case：1,200 個最大分片逆序組裝，以及完成後 100,000 次重複封包，通過。
- 原生解碼測試涵蓋 H.264／HEVC GOP 順序、尺寸重設及關閉後像素所有權。
- 分片所有權／零配置重複封包與原生解碼核心案例，以 race detector 重複 20 次：通過。
- `GOEXPERIMENT=cgocheck2` 執行原生解碼所有權及擷取 buffer 測試：通過。
- 原生擷取使用人工 pixel buffer 測試空畫面、關閉、尺寸錯誤、BGRA→RGBA 色彩，以及 take 後關閉擷取再 copy。此測試不需要螢幕錄製權限。
- 實際 `stream-smoke -live -frames 10 -rounds 1 -fps 10 -codec hardware-h264` 被 macOS TCC 拒絕，未完成；沒有繞過權限或宣稱實際桌面驗證通過。

## 後續範圍

本輪沒有實作跨階段像素池、縮小顯示鎖、Core ML／補幀快照節流、GPU texture 直通，亦未改變不完整分片的逾時策略。這些涉及額外所有權／恢復協定設計，應分輪以 CPU、配置 profile、RSS 與 p95 延遲驗證。
