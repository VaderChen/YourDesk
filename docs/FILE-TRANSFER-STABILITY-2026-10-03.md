# 檔案傳輸穩定性與最佳化（2026-10-03）

本輪維持既有視窗、按鈕、操作流程與功能範圍。未變更 HTML、CSS、翻譯、4 KiB 區塊、2 GiB／64 項上限、指令格式、不覆寫規則或手動續傳方式；保留前一輪影像／聲音最佳化。

## 穩定性修正

### 上傳取消與暫存回收

原本刪除或查詢暫存檔失敗時，仍可能回覆取消成功，並移除續傳憑證與額度追蹤，使殘留檔案失去重試清理的機會。新增測試先在原版重現取消、到期回收、暫存查詢及寫入失敗後清理等四種故障，再確認修正。

- 刪除／查詢失敗時保留暫存身分、憑證及額度，取消回報失敗；重連後可再次取消，到期回收也可重試。
- 記錄建立時的檔案身分，寫入 handle 異常關閉後仍能確認及取消原始暫存。外部替換的檔案不視為原傳輸檔案。
- 清理確認後才關閉資源、釋放額度。提交成功的收據及其他平台硬連結備援仍沿用既有行為，不把已完成檔案重新變成待傳工作。

### 下載取消與視窗關閉

原本本機暫存刪除失敗會被忽略，原生下載 Promise 已結束，頁面也會視為取消完成。

- 清理失敗時保留下載工作、目錄及原始檔案身分，沿用現有取消失敗狀態，使用者可再次按取消。
- 不把未完成清理的下載名額交給下一個工作；關窗仍沿用既有 8 秒上限，未確認清理時保留視窗供重試。
- 清理等待由明確取消要求喚醒，不自行密集重試。父視窗／程序結束會終止等待並關閉 handle，避免 worker 阻擋退出。
- 取消期間保留原檔案 handle；提交前以 Unix `F_DUPFD_CLOEXEC`／Windows `DuplicateHandle` 複製現有 handle，保留原先 writer close 錯誤檢查，且不重新依路徑開啟可能被替換的特殊檔案。
- 暫存已不存在視為完成清理；外部替換檔案保留。已成功下載的檔案不因取消而被刪除。

### 選檔掃描期間斷線／重連

選檔可讀性檢查及資料夾掃描是非同步工作。原本掃描尚未完成就切換工作階段，完成後可能自動使用新連線開始舊批次上傳。現在保存掃描開始時的 instance；中途斷線或換 instance 時，保留已選 File 並標記中斷，等待使用者按續傳。沒有新增自動重試或新的操作步驟。

## 效能與記憶體

- 上傳在既有共用 mutex 保護下重用 4,098-byte Base64 解碼暫存，寫入及驗證順序不變，仍拒絕超過 4 KiB 的解碼內容。
- 每個下載重用解碼緩衝；每次同步寫入後才重用，仍驗證實際長度、位移、EOF 與來源版本。
- 目錄排序逐 rune 比較小寫字元，避免每次排序比較建立新字串。大小寫、Unicode、資料夾優先、原名同序判斷、隱藏過濾、排序上限及分頁維持原樣。
- 每個區塊的指令能力查詢直接在原 mutex 下讀取受限清單，省去複製；公開 `RemoteCommands` 仍回傳獨立副本。
- 純上傳進度只更新當列與總進度，省去整份列表的控制狀態掃描。開始、暫停、取消、斷線及完成等狀態變化仍完整更新按鈕與路徑保護。

Apple M4、darwin/arm64、Go 1.27.1、`GOMAXPROCS=4`；同一 benchmark 以修改前原始碼 overlay 與新版本各執行三次，取配置中位數：

| 情境 | 修改前 | 修改後 | 改善 |
| --- | ---: | ---: | --- |
| 4 KiB 上傳完整 handler | 約 28,237 B/op；21 allocs | 約 23,363 B/op；20 allocs | 配置量減少 17.3% |
| 1 MiB 完整本機 mock 下載 | 約 7,040,679 B/op；16,864 allocs | 約 5,795,529 B/op；16,598 allocs | 配置量減少 17.7% |
| 列舉／排序 2,048 個混合大小寫項目 | 約 4,854,799 B/op；79,113 allocs | 約 2,540,814 B/op；30,905 allocs | 配置量減少 47.7% |
| 每次檔案指令能力查詢 | 256 B/op；1 alloc | 0 B/op；0 alloc | 移除清單副本 |
| 1,000 筆遠端清單、每個 4 KiB ACK 的列表 DOM 查詢 | 2,002 次 | 0 次 | 純進度不再掃描整表 |

上傳與下載量測包含 JSON、位移驗證及本機 I/O；下載經隔離 HTTP mock。目錄量測包含檔案系統列舉。數字表示每次操作產生的 Go heap 配置，不是程序 RSS；固定暫存會保留少量記憶體供重用。系統並行編譯使耗時波動，不主張端到端傳輸速度倍率或高延遲網路吞吐量提升。

## 驗證

- 專案標準 Opus 設定的完整 Go 測試通過。
- `internal/filetransfer`、`internal/clientui`、`internal/p2p`、`internal/hostsession` 完整 race 測試通過；下載清理的補充邊界另有針對性 race 驗證。
- 57 項 JavaScript 回歸通過，涵蓋取消失敗重試、限時關窗、掃描跨工作階段、純進度更新與按鈕等價。
- Edge 四語系瀏覽器 smoke 通過，包含選檔／目的目錄、分頁與複選、ACK 遺失、舊工作階段回覆、暫停／續傳及關窗流程；檢查淺／深色截圖。
- 下載測試涵蓋 HTTP body 中斷後手動續傳、錯誤／過大／不完整區塊拒收、暫存身分與內容保護。
- 上傳故障測試使用隔離目錄及真實 OS 權限錯誤；取消查詢失敗、刪除失敗、寫入 handle 異常及到期回收均保留可追蹤狀態。
- Windows amd64、`CGO_ENABLED=0` 的 filetransfer／clientui 測試程式交叉編譯通過；這僅確認 Go 邏輯及 Windows handle helper 可編譯，不等於 Windows 原生實機驗收。
- `gofmt`、JavaScript 語法與 `git diff --check` 通過。原有原生編譯警告仍存在，無新增測試失敗。

重跑入口：

```sh
export GOCACHE=/private/tmp/yourdesk-go-build-cache-20261003
export GOMAXPROCS=4
python3 scripts/opus.py darwin/arm64 go test -p 2 ./... -timeout=180s
go test -p 2 -race ./internal/filetransfer ./internal/clientui ./internal/p2p ./internal/hostsession -count=1 -timeout=180s
node scripts/test_files_ui.js
FILES_BROWSER_CHANNEL=msedge node scripts/smoke-files-ui.cjs

go test -p 2 ./internal/filetransfer -run '^$' -bench '^(BenchmarkUploadWriteChunk|BenchmarkVisibleDirectory)$' -benchmem -count=3
go test -p 2 ./internal/clientui -run '^$' -bench '^BenchmarkFileDownloadBuffers$' -benchmem -count=3
go test -p 2 ./internal/p2p -run '^$' -bench '^BenchmarkSupportsFileCommand$' -benchmem -count=3
```

瀏覽器 smoke 需可載入既有 Playwright；本次透過命令級 `NODE_PATH` 使用本機快取，沒有變更專案相依套件或全域 Go 設定。原始紀錄保存在 `.local-run/filetransfer-20261003/`，較早的個別測試紀錄亦保留在 `.local-run/`；本輪 `.bak` 成功後清除，原先已存在的備份保留。

尚未執行 Windows 實機、跨機長時間傳輸、真實滿磁碟及 Finder／Explorer 原生操作驗收，亦未重新打包或發布安裝包。UI mock 與本機回歸不代替這些驗收。

以上為實作階段的量測及驗證記錄；後續跨平台套件、簽章與發布範圍見 [1.26.1003 build 1231 發行說明](RELEASE-1.26.1003-build-1231.md)。Windows 發布及更新使用 Portable ZIP，登入前服務另附同版服務 ZIP。
