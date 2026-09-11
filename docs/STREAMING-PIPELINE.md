# 三階段雙緩衝串流與本機 Smoke

擷取、縮圖／編碼、傳輸改為三個獨立 worker，由 `internal/streampipeline` 共用模組串接。各交接點最多兩個工作槽位：擷取可與上一張編碼重疊，編碼可與上一張傳送重疊。這是工作所有權的雙緩衝；擷取 API 仍配置獨立像素記憶體，尚非零拷貝或固定像素記憶體池。

## 正式系統啟用方式

一般 Client 啟動已直接使用三階段雙緩衝；macOS 被控端預設採用持續擷取，不需要額外開關或 smoke 參數。`runUITest.command` 會編譯並啟動相同的正式程式路徑。更新並重新啟動被控端 Client 後，新連線即可使用；已在執行的舊版程式不會因重新編譯而自動更新。Windows 沿用既有擷取方式，仍接入共用三階段管線。

「串流方式」中的「自動配置/延遲優先」及「自動配置/流量優先」使用目前連線的量測資料調整 FPS／GOP，保留畫質、解析度、碼率與影像傳輸設定。持續擷取會遵循設定後的來源 FPS 上限，無須另外選擇擷取策略。傳輸協定未變更，此項擷取改善不要求 遠端顯示 同步更新。

## 順序與資源

- 每階段只有一個 worker；JPEG 同一影格的區塊整批交付，H.264／HEVC 保持 GOP 編碼及傳輸順序。
- 緩衝滿時反壓上游，既有擷取 ticker 自然略過未處理的節拍，不累積無界工作，也不任意丟掉已編碼的 P 幀。
- 來源尺寸及螢幕切換使用世代識別，舊螢幕已排隊的畫面可捨棄，新螢幕重新建立編碼／JPEG 差異基底。
- 傳送回報壅塞時提升恢復世代，捨棄舊世代排隊內容，要求下一輪重新建立編碼器與完整 JPEG 基底。既有 SendFrame 呼叫語意保持相容，管線使用會回報 JPEG 丟幀的 SendFrameChecked。
- FPS 更新以原子值通知擷取階段；其他串流設定及編碼器、縮圖器由編碼階段單獨持有。線上協定不變。
- 結束或出錯時取消工作，等待所有 worker 結束後才關閉原生編碼與縮圖資源。無法中斷的原生呼叫須等返回。

## 遠端顯示 接收與解碼雙緩衝

遠端顯示 已將收封包與解碼分成兩階段：WebRTC 接收回呼負責既有的封包重組，完整 JPEG 區塊或 H.264／HEVC 影格透過共用 `streampipeline.Consumer` 交給單一解碼 worker，再更新顯示畫面。解碼不再直接執行於收件回呼，因此可與下一幀的接收／重組重疊。

交接使用兩個槽位，包含正在解碼的項目；收件端正在組裝或等待交付的一幀，以及底層網路緩衝，不計入這兩個槽位。滿載時等待空槽，不覆寫已排隊資料，維持 JPEG 區塊順序及影片 GOP 相依關係。既有序號中斷後等待關鍵幀的處理保留，並未新增丟幀策略。

關閉 遠端顯示 時先取消交接、解除等待，等當前解碼完成後才關閉連線及原生解碼器；尚未處理的佇列不再解碼。解碼統計只計算解碼 worker 的處理時間，不包含等候空槽的時間。此改動適用於 macOS／Windows 遠端顯示，不變更傳輸協定；更新接收端即可使用。接收端本機正確性測試已完成，尚未量測接收端實際效益；下方效能數字皆為先前傳送端測試結果。

## 2026-09-10 本機比較

Apple M4 Pro，macOS 26.6.2。真實桌面擷取，統一縮圖至 1920×1080，編碼品質 80；H.264 使用 GOP 10、12 Mbps 目標。每輪先暖機兩幀，再處理 40 幀；串行與雙緩衝交錯兩輪，各模式共 80 幀。未限制擷取 FPS，用來觀察此工作負載的最大吞吐量。桌面內容可能隨實際操作改變，兩輪不足以宣稱普遍效能。

傳輸使用本機 loopback TCP，逐幀檢查序號及 CRC，收到確認才算送達。不是正式 WebRTC 跨機測試；延遲從擷取呼叫開始算到本機接收確認，包含排隊，不含 遠端顯示 解碼、顯示或遠端網路。JPEG 使用完整 JPEG 編碼，未套正式串流的差異區塊，不能直接當作 JPEG 遠端桌面的 FPS。

| 模式 | 串行 → 雙緩衝 FPS | 吞吐變化 | 平均單幀延遲 | 每輪 P95 的平均 |
| --- | --- | --- | --- | --- |
| 硬體 H.264 | 9.09 → 9.38 | +3.2% | 110.1 → 115.9 ms | 116.7 → 119.7 ms |
| 軟體 JPEG | 8.84 → 9.42 | +6.5% | 113.1 → 128.2 ms | 117.2 → 131.1 ms |

本次主要瓶頸是擷取：H.264 串行約 100.7 ms、雙緩衝約 106.4 ms；編碼約 9.2～9.3 ms，傳輸約 0.15～0.17 ms。JPEG 擷取約 90.3 → 105.6 ms，編碼約 22.3～22.6 ms。並行後擷取耗時也有增加，因此未取得理論上完整的重疊效益；原因尚未進一步確認，不能直接歸因於特定硬體資源。

**結論：吞吐小幅改善，但本機單幀延遲沒有下降。** 下一步應量測及改善原生擷取路徑，而不是再增加緩衝深度。雙緩衝先提供可分別優化三階段的結構。

所有 320 幀均完成 loopback 順序及 CRC 檢查；未做影像解碼或畫質驗證。管線 smoke 另以 race detector 檢查正常排空、略過空幀、傳送失敗取消、worker 結束及緩衝上限。本次並非跨機 GOP 丟幀恢復實測。

## 重現

```
go build -o /tmp/yourdesk-stream-smoke ./cmd/stream-smoke
scripts/sign-local.sh /tmp/yourdesk-stream-smoke
/tmp/yourdesk-stream-smoke -frames 40 -rounds 2 -codec hardware-h264
/tmp/yourdesk-stream-smoke -frames 40 -rounds 2 -codec software-jpeg
go test -race ./internal/streampipeline
```

工具只使用本機回送，不保存桌面影像。需要有效螢幕錄製權限；原始測量 JSON 留於本機 reports/stream-pipeline。

## 持續擷取改善

檢查原擷取套件後發現，macOS 每次 Capture 都重新呼叫 SCShareableContent 查詢，再用 SCScreenshotManager 進行一次截圖。新版 macOS 改為持續 SCStream，螢幕與尺寸未改變時重用工作階段，原生回呼只保留最新完整畫面。固定最高 60 FPS，符合目前來源設定範圍；消費端仍依使用者 FPS 上限取樣。ScreenCaptureKit 內部 queueDepth 使用官方最低值 3，這與 Go 階段間的雙緩衝槽位不同。

回呼交接保留 CVPixelBuffer，複製時再額外保留，完成後釋放；輸出轉為既有 RGBA 以相容編碼器。仍有 CPU 記憶體複製，不宣稱零拷貝或純硬體擷取。沒有新完整幀時回報正常略過，不重送舊幀冒充 FPS。啟動或執行失敗暫用原截圖路徑，30 秒後才重試；螢幕／尺寸變更重建 session，關閉時移除回呼並停止擷取。Windows 仍使用原擷取路徑，尚未有 Windows 實機效能結論。

同機、1920×1080、每輪暖機兩幀與測量 40 幀、交錯兩輪。下表均為雙緩衝：

| 路徑 | 實際 FPS | 擷取呼叫平均 | 擷取呼叫至本機收件確認 |
| --- | --- | --- | --- |
| 原截圖／H.264，不限速 | 9.38 | 106.40 ms | 115.86 ms |
| 持續擷取／H.264，不限速 | 57.19 | 17.23 ms | 27.84 ms |
| 持續擷取／H.264，上限 20 FPS | 19.81 | 2.36 ms | 16.12 ms |
| 持續擷取／H.264，上限 30 FPS | 29.65 | 1.06 ms | 14.02 ms |
| 原截圖／軟體 JPEG，不限速 | 9.42 | 105.65 ms | 128.21 ms |
| 持續擷取／軟體 JPEG，不限速 | 45.13 | 2.20 ms | 43.94 ms |

H.264 不限速約由 9.38 提升到 57.19 FPS，擷取呼叫到本機收件確認約由 115.86 降到 27.84 ms。20／30 FPS 設定均已接近目標。這些是短時間本機 smoke 結果，無跨網路或 遠端顯示 顯示測量。持續擷取的呼叫時間包含等待新幀與記憶體複製；回呼產生影格到呼叫取用前的時間未另行計入，因此不能直接當成使用者看到畫面的端到端延遲。

持續擷取四組共 640 幀通過 loopback 序號與 CRC 檢查，重複建立／停止 session 均完成。桌面持續有更新；靜止桌面、休眠喚醒及多螢幕熱插拔尚未完整實機測試。原生 session 限制時間避免無限等待，沒有新影格不應被視為可用頻寬或效能不足。

此次優先解決每幀重建截圖的負擔，尚未導入分段擷取或 Slice 壓縮；在硬體 H.264 已接近 60 FPS 的本機結果下，還沒有證據需要增加這些複雜度。原 JPEG 差異區塊仍保留。

重現時在同一工具加上 `-live`，可選 `-fps 20` 或 `-fps 30`。未加 `-live` 即使用原截圖基準。

官方參考：[ScreenCaptureKit 範例](https://developer.apple.com/documentation/screencapturekit/capturing-screen-content-in-macos)、[queueDepth](https://developer.apple.com/documentation/screencapturekit/scstreamconfiguration/queuedepth)、[minimumFrameInterval](https://developer.apple.com/documentation/screencapturekit/scstreamconfiguration/minimumframeinterval)。

## 接收端本機正確性驗證（2026-09-10）

在本機 macOS 執行下列測試，各重複三輪，全部通過，Go race detector 未回報競態：

- 交接順序：每輪 1,000 筆，確認不遺漏、不重排。
- 滿載與關閉：兩槽占用後第三筆等待；關閉可解除提交等待，且會等當前解碼完成，取消後不處理排隊影格。
- 封包重組：每輪 100 幀，模擬片段倒序、重複及一幀不完整；99 幀正確交付。重用封包記憶體不影響已交接的資料。
- 實際編解碼：JPEG、H.264、HEVC 各 24 幀，640×360 移動方塊；影片 GOP 10，各含 21 張非關鍵影格。串行與雙緩衝解碼的每幀尺寸及像素 CRC 一致，三輪共完成 432 次影格解碼。

```sh
go test -race ./internal/streampipeline ./internal/video -run 'TestConsumer|TestReceiveDoubleBufferNativeDecode' -count=3 -timeout=90s -v
go test -race ./internal/p2p -run 'TestAssemblerDoubleBufferOwnership|TestFrameAssembler' -count=3 -timeout=30s -v
```

本次未發現需要修改正式流程的錯誤，新增測試留作回歸驗證。封包重組與原生解碼分別驗證，未啟動完整 WebRTC／遠端顯示 視窗，亦未涵蓋 Windows 原生解碼或跨機網路；結果不代表已量測端到端延遲改善。

### Windows 接收端靜態檢查

2026-09-10 檢查 Windows 遠端顯示 的雙緩衝與原生解碼銜接，未發現本次拆分引入的執行緒或生命週期衝突；Windows x64、ARM64 遠端顯示 均通過 CGO 交叉編譯。

- `winmedia.Session` 以 `runtime.LockOSThread` 固定 COM／Media Foundation／D3D11 工作執行緒，建立、操作與關閉原生資源皆在該執行緒；外層解碼 worker 不直接操作 COM。
- Session 的呼叫與關閉由互斥鎖串行保護；遠端顯示 先停止雙緩衝 worker，再釋放解碼器。新增交接不會建立多個並行解碼呼叫。
- 解碼結果使用 `C.GoBytes` 複製後才釋放原生輸出，已交付的像素不會被下一輪覆寫。JPEG 使用既有軟體路徑，Windows HEVC 仍未支援。
- 既有原生事件輪詢部分設有 2 秒等待期限，但不能強制中止阻塞中的驅動／COM 呼叫，因此關閉時間仍取決於原生呼叫返回。

此為程式碼審查與交叉編譯，未在 Windows 實機執行，不代表已驗證各 GPU 驅動的解碼效能或關閉延遲。

### Windows 發送端靜態檢查

2026-09-10 檢查擷取、縮圖／編碼、傳送三階段，並修正既有 GDI 執行緒風險：`ScreenshotCapturer.Capture` 在 Windows 固定整次呼叫的 OS 執行緒，直到擷取套件的原生資源清理完成才解除。依 [Microsoft ReleaseDC 文件](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-releasedc)，GetDC 與 ReleaseDC 必須在同一執行緒呼叫；原擷取套件未自行固定 Go 執行緒。

擷取輸出使用獨立 RGBA；縮圖緩衝僅由編碼階段使用；Media Foundation 編碼與 D3D11 縮圖各由固定 COM 執行緒持有。編碼結果複製為 Go 位元組後才交给傳送階段，下一輪重用像素緩衝不會覆寫已排隊的壓縮資料。三階段結束後才關閉編碼器與縮圖資源；壅塞恢復保留既有世代檢查及 GOP／JPEG 基底重建流程。

修正後 Windows x64、ARM64 Client 均通過 CGO 交叉編譯。Windows 仍使用 GDI 截圖，未改成 macOS 的持續擷取機制；此次未做 Windows 實機測試，未宣稱擷取效能提升。

### Windows GDI 擷取修正與資源重用

2026-09-10 進一步檢查原 screenshot 套件，發現每幀重建 DC／compatible bitmap、配置 GlobalAlloc 中介空間，經 GetDIBits 複製後再轉 RGBA。此外，呼叫 GetDIBits 時 bitmap 仍選在 memory DC 中，不符合 [Microsoft GetDIBits 規範](https://learn.microsoft.com/en-us/windows/win32/api/wingdi/nf-wingdi-getdibits)。這是 API 使用問題，但未經 Windows 計時，不能認定它就是延遲主因。

正式 Windows Client 現改用持續 GDI 工作階段：

- 固定 OS 執行緒持有及清理 DC、DIB；尺寸與位置未變時重用資源。
- BitBlt 直接寫入 top-down 32-bit DIB，GdiFlush 完成同步後讀取像素，移除 GetDIBits 與 GlobalAlloc 中介複製。參考 [CreateDIBSection 文件](https://learn.microsoft.com/en-us/windows/win32/api/wingdi/nf-wingdi-createdibsection)。
- 每幀仍配置獨立 RGBA 並轉換 BGRA，確保雙緩衝的編碼階段不被下一次擷取覆寫；不宣稱零拷貝。
- 畫面位置／尺寸改變時重建；擷取失敗先釋放資源，下一次重建重試。關閉等待當前擷取返回，再在原執行緒清理。
- 單次 ScreenshotCapturer 也使用相同 DIB 實作，避免回到有問題的 GetDIBits 路徑；只有 LiveCapturer 重用跨幀資源。

Windows x64／ARM64 Client 交叉編譯通過。尚未實機量測 BitBlt、同步等待與 RGBA 轉換各自耗時，也未驗證休眠／鎖屏、混合 DPI 或多 GPU。螢幕列舉與全畫面 CPU 讀回仍存在；此次不是 DXGI Desktop Duplication，也不能套用 macOS 的效能數字。

### Windows DXGI 硬體擷取

正式 Windows Client（CGO 版本）預設優先使用 DXGI Desktop Duplication，再接既有擷取／編碼／傳送雙緩衝。依目標螢幕的桌面座標尋找對應 DXGI output 與 adapter，在該 GPU 建立 D3D11 裝置，避免混合顯示卡時固定選錯 GPU。固定 OS 執行緒持有 duplication、context 與可重用的 staging texture。

AcquireNextFrame 等待上限 16 ms；沒有新畫面或只有硬體游標更新時正常略過，首張有效影像仍交付。每次成功取得影格都以 RAII 保證 ReleaseFrame，映射後亦保證 Unmap。依 RowPitch 逐列轉換 BGRA 為獨立 RGBA，避免 GPU／下一幀覆寫正在編碼的資料。

不支援、裝置遺失或擷取出錯時關閉 DXGI 工作階段，回退持續 GDI，30 秒後重試；螢幕座標或尺寸改變時立即重新選擇。旋轉螢幕目前使用 GDI；無 CGO 版本亦使用 GDI。紀錄檔會顯示硬體擷取啟用或退回 GDI 的 HRESULT。

這版包含 GPU 擷取，但仍有 GPU→CPU staging 讀回與像素轉換，尚未將 DXGI texture 直接交給 Media Foundation，也未利用 dirty／move rectangles。16 ms 是取得新幀的等待上限，不包含驅動 Map／GPU 同步耗時。獨立硬體游標不額外合成，沿用 遠端顯示 游標；系統已合成進桌面的游標可能仍出現在擷取中。

Windows x64、ARM64 Client 交叉編譯均通過。尚無 Windows 實機 FPS／延遲數字；待驗證不同 GPU、混合 DPI、鎖屏／喚醒、熱插拔與回退恢復。

官方依據：[Desktop Duplication API](https://learn.microsoft.com/en-us/windows/win32/direct3ddxgi/desktop-dup-api)、[DuplicateOutput](https://learn.microsoft.com/en-us/windows/win32/api/dxgi1_2/nf-dxgi1_2-idxgioutput1-duplicateoutput)、[AcquireNextFrame](https://learn.microsoft.com/en-us/windows/win32/api/dxgi1_2/nf-dxgi1_2-idxgioutputduplication-acquirenextframe)。
