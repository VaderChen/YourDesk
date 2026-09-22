# 遠端聲音

進階設定在「長時間未操作自動關閉」下方提供「開啟遠端聲音」，預設關閉。啟用後播放遠端電腦的系統輸出，不擷取麥克風；偏好會保存，已開啟的顯示區域會隨設定更新。兩端皆需支援此功能，舊端會顯示不支援原因，畫面與鍵鼠仍可繼續使用。

## 編碼與品質

設定頁「串流方式」並列「影像輸出編碼」與「聲音輸出編碼」。聲音輸出編碼可在遠端聲音關閉時先選好，預設仍為 Opus；實際播放開關保留於進階設定，改選編碼不會自行開啟聲音。

聲音編碼選單分為「可用」與「不支援」，依本機實測的解碼能力分組，不支援的項目停用；遠端是否可編碼仍在連線時確認。AAC 硬體可回退軟體，不因本機沒有硬體加速就歸為不支援。尚未取得能力結果時顯示「聲音能力偵測中…」，暫停選取；偵測完成後更新分組，保留已保存的編碼，不自動改成其他格式。

預設仍為 Opus；選單依序顯示「自動 (按排列順序優先)」、AAC 硬體、Opus 軟體、AAC 軟體、PCM 未壓縮。自動模式透過已認證的 `audio.capabilities` 取得來源實測能力，與本機解碼能力取交集，依 **AAC 硬體 → Opus → AAC 軟體 → PCM** 選擇；AAC 第一順位必須有來源硬體編碼實測成功。實際送出的設定及接收端均使用協商後的具體格式，保留世代隔離。舊遠端未提供自動協商時提示更新，可手動選擇既有格式。未設定編碼時採 Opus，既有明確保存的 AAC／PCM 選擇不變。背景能力分析使用合成 PCM 實際編解碼，不開啟錄音或播放裝置。Opus 使用內嵌 libopus 軟體編解碼，不宣稱硬體加速。AAC 硬體僅在實測成功時優先選用，建立或處理失敗時回退軟體；解碼端獨立優先使用可用的 AAC 硬體解碼。AAC 軟體選項只限制遠端編碼，不停用本機硬體解碼。

聲音品質跟隨**各顯示區域既有的影像品質選單**，不新增另一個聲音品質選單：

| 顯示區域選項 | Opus 目標碼率 | AAC 目標碼率 |
| --- | --- | --- |
| 低流量 | 48 kbps | 96 kbps |
| 標準 | 96 kbps | 128 kbps |
| 高畫質 | 160 kbps | 192 kbps |

Opus 使用 48 kHz 雙聲道、20 ms／960 取樣框，受限可變碼率、音訊模式、複雜度 5；碼率是音訊目標值，不含封包和網路協定開銷，20 ms 並非端到端延遲。AAC 使用 LC、48 kHz、雙聲道、每包 1024 個取樣框。PCM 是相容用的未壓縮格式，固定 48 kHz／16 位元／雙聲道，約 1.536 Mbps；品質選單不壓縮 PCM。切換編碼或品質時重建聲音工作階段，清除舊音訊，可能有短暫靜音。

## 原生後端

- macOS 13 以上：ScreenCaptureKit 擷取系統聲音、libopus／AudioToolbox 編解碼、AudioQueue 播放。沿用系統螢幕／音訊錄製授權；權限不足時回報錯誤，不修改 TCC。擷取排除擷取程序自己的聲音。
- Windows：WASAPI loopback 擷取預設輸出裝置、libopus／Media Foundation AAC 編解碼、WASAPI 播放；以共用模式轉換成統一 PCM 格式。先列舉硬體 MFT 並實測，再使用軟體 MFT。原生資源的建立、操作和釋放固定在同一 OS 執行緒。
- 其他平台目前不提供原生音訊。擷取與播放使用作業系統元件；Opus 靜態連結，不增加外部音訊執行檔或編解碼 DLL。

播放／擷取裝置在開啟時選擇預設裝置；若作業系統切換裝置使原資源失效，會停止聲音並顯示錯誤，可關閉再開啟此開關以重新選擇。安全桌面、未登入工作階段及個別裝置驅動的收音能力仍由作業系統決定。

## 傳輸及生命週期

已認證的桌面連線才提供 `audio.configure`，檔案專用連線不接受音訊。`audio-v1` 使用有序、不重傳的獨立 DataChannel。接收回呼只排入最多八包的佇列，編解碼、播放及擷取均由背景工作者執行；影像或控制通道壅塞時直接捨棄該音訊，不等待回呼或持續堆積。

每次啟停、品質或編碼改變會提高世代編號；舊世代、格式錯誤、重複或過大的封包不進入解碼器。中途缺包時清除播放佇列與解碼歷史。每秒更新擷取租約，六秒沒有收到更新即停止來源；關閉顯示區域與斷線會取消背景工作並釋放裝置。預設關閉時不建立擷取或播放裝置；背景編解碼能力探測只使用合成資料。

聲音錯誤在顯示區域及主畫面提示，與整條桌面連線的失敗狀態分開。工具列的編碼狀態泡泡先顯示「影像編碼／聲音編碼」，分隔線下方顯示「影像解碼／聲音解碼」；Mac 與 Windows 共用此順序。編碼欄位保留硬體／軟體及格式；影像、聲音解碼只顯示硬體／軟體。來源與接收之間的分隔線上下等距。聲音欄位依實際來源回報與已建立的解碼器顯示，包含硬體失敗後的軟體備援；未啟用顯示「關閉」，尚未收到聲音或資源釋放後顯示「等待聲音」，錯誤顯示「無法使用」。狀態更新在背景且只發布變更，不讓 UI 等待編解碼。

編碼實際後端也記錄在執行記錄；設定頁依目前選取的格式顯示本機解碼能力，編解碼分析頁顯示編碼與解碼實測結果。

## 驗證紀錄（2026-09-22）

- Opus 在三個碼率各連續編解碼 100 包／2 秒，驗證每包 3840 bytes 雙聲道 PCM、非靜音取樣及無效封包拒絕。實測平均 48.88／97.232／160.48 kbps。Mac 原生 AAC 三個碼率也通過回歸；本機 Apple M4 Pro／macOS 27 偵測到軟體 AAC，未宣稱硬體 AAC 可用。
- 使用固定 Developer ID 簽署的隔離 Smoke APP，在已授權的測試環境完成系統聲音擷取、AudioQueue 播放，以及擷取 → Opus／AAC／PCM → 有界佇列 → 解碼 → 播放。Opus 三檔及 AAC／PCM 切換共處理 594 包，關閉後停止傳送；只統計合成測試聲音，不儲存錄音。
- 實際本機 WebRTC 通道測試確認慢速聲音接收不阻塞控制 ping、佇列有上限，檔案專用工作階段拒絕傳送；Go race 測試涵蓋聲音、P2P 與 Host。
- 真實瀏覽器 Smoke 驗證預設關閉、預設 Opus、設定保存、明確保存的 AAC 不被覆寫、編碼切換時同步能力顯示及不另設品質選單。
- Mac Client／顯示區域完整影音建置、Windows x64 Client／顯示區域，以及 Windows ARM64 原生音訊測試程式交叉編譯通過。Windows 實際 WASAPI／Opus／AAC／播放、Mac ↔ Windows 音訊互通及長時間裝置切換尚待實機驗證。

可執行 `python3 scripts/opus.py darwin/arm64 go test ./internal/remoteaudio ./internal/p2p ./internal/hostsession ./cmd/remote` 及 `scripts/smoke-audio-settings.cjs`。`YOURDESK_AUDIO_SMOKE=1` 才會開啟原生裝置測試；Mac 裝置 Smoke 必須由已簽署且具系統錄製權限的 APP 執行，未授權的臨時 Go test 執行檔會被 TCC 拒絕。

## 建置

桌面建置入口 `buildMac.command`、`buildWin.command`、`runUITest.command`、`make build` 均包含 `opus` build tag。`scripts/opus.py` 固定使用 libopus 1.6.1，下載後核對官方 SHA-256，依目標平台建立靜態庫並檢查個人路徑；快取放在 `.local-run/opus`。ARM64 使用固定 NEON 路徑，避免工具鏈缺少執行期 CPU 探測支援；Windows 明確指定最低 SDK 為 Windows 10。不啟用 DRED／OSCE／深度學習 PLC；目前接收流程未啟用 Opus FEC 或缺包補償。

只測音訊可用 `python3 scripts/opus.py 平台/架構 go test|build ...`；完整影音建置用 `scripts/ffmpeg.py`。直接執行未加 tag 的 `go test` 可測共用邏輯，但不包含 Opus 原生測試；該精簡建置啟用 Opus 時會明確回報未包含編解碼器。發行包及 Mac APP 資源包含 `ThirdPartyLicenses/Opus` 的授權與版本來源資料。聲音功能仍預設關閉。

API 依據：[Opus 1.6.1 來源及雜湊](https://opus-codec.org/release/stable/2026/01/14/libopus-1_6_1.html)、[Opus Encoder API](https://opus-codec.org/docs/opus_api-1.6/group__opus__encoder.html)、[Apple ScreenCaptureKit](https://developer.apple.com/documentation/screencapturekit)、[Microsoft AAC Encoder](https://learn.microsoft.com/en-us/windows/win32/medfound/aac-encoder)、[Microsoft AAC Decoder](https://learn.microsoft.com/en-us/windows/win32/medfound/aac-decoder)。

本輪自動編碼測試依序驗證 AAC 硬體、Opus、AAC 軟體、PCM 的能力回退，以及兩端無共同格式時明確拒絕；瀏覽器 Smoke 驗證選單順序、儲存 auto、分組、解碼摘要與分隔線等距。相關檔案傳輸／聲音測試、針對新增邏輯的 race 測試及 Mac arm64／Windows amd64 Client、顯示區域建置通過；未將編譯結果視為跨機音訊實測。
