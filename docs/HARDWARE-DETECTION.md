# 編解碼分析與背景偵測

AV1 更新：已補上 Windows／macOS 軟體編碼、macOS VideoToolbox 硬解與軟解備援；編解碼分析頁面隱藏 128×128，但保留內部快速探測。最新支援範圍、建置與驗證限制見 [AV1 編解碼](AV1.md)。

APP 取得單一執行個體鎖後，啟動獨立背景協調工作；不等待偵測完成才開啟視窗。偵測結果可提供新連線的能力篩選與差分記憶體策略，不改寫使用者偏好，不切換現有連線的編解碼器。

## 執行與生命週期

- `internal/hardwareprobe` 每個 APP 程序只啟動一次，最多 4 個偵測子程序同時運作。
- 重用已打包的 `yourdesk-client --hardware-probe ...`，不依賴 `try`、外部編譯器或額外 detector 執行檔。
- helper 在 CLI 旗標解析、UI／登入前服務及單一執行個體鎖之前處理，完成後直接返回，不遞迴啟動 APP。
- 任務參數只接受內建組合，沒有任意檔案或命令參數。
- 每項 8 秒逾時，APP 結束或 Run 返回時取消工作並結束進行中的子程序。未執行項目標成 cancelled。
- 正常退出時最多等候 2 秒讓子程序監督者完成清理；此等待僅在退出路徑，不影響開啟流程。
- 子程序不接收 stdin 或密碼管線；沿用 Windows 無額外主控台視窗的啟動方式。
- 不將原始程序錯誤或驅動訊息顯示成 toast。只寫整體狀態及耗時至日誌。
- 此上限適用於新的 detector 工作佇列；既有超解析度與影片能力探測維持原有生命週期。

## 平台範圍

### macOS（CGO）

基本資訊：CPU／OS、公開 SIMD 能力、核心／記憶體、電源／熱狀態、Metal 裝置、VideoToolbox 編碼器清單，以及 JPEG／H.264／HEVC／AV1 硬解 API 宣告。

實際驗證 JPEG／H.264／HEVC × BGRA／NV12 full range／NV12 video range × 128×128／1920×1080，共 18 組，每組拆成編碼與解碼，加上基本查詢共 45 項（含新增的 AV1 硬體／軟體各兩種尺寸、兩個方向，共 8 項）。編碼使用合成影像；解碼使用內嵌固定樣本，驗證指定輸出格式與尺寸，兩個方向各有獨立 helper 與逾時。

CPU 查詢不存在、硬體狀態屬性不支援等情況保留 `null` 與 OSStatus，不能當作 false。例如目前實驗機 JPEG 的硬體使用狀態屬性不支援；編解碼可完成，不代表硬體狀態已取得確認。

### Windows／其他平台／macOS 無 CGO

使用 `golang.org/x/sys/cpu` 查詢相應架構公開的 SIMD 能力，並透過現有正式編解碼介面驗證 JPEG／H.264／HEVC 的 RGBA 輸入、128×128／1920×1080，共 6 組加基本查詢；Windows 另加入 AV1 的兩組 RGBA 實測。

Windows CGO 另外執行下述原生矩陣。只回報真正執行的後端；JPEG 若使用軟體 fallback 會明確回報。未實作的 codec 標成 unavailable，沒有硬體狀態回報介面時保留 unknown／null。不把 RGBA 包裝介面成功解讀成底層硬體接受 BGRA 或 NV12。Windows 從登錄取得 CPU 型號，透過 EnumDisplayDevicesW 取得顯示卡名稱（排除鏡像驅動並去除重複名稱），沿用前端 cpu.brand 與 gpus[].name 欄位；查詢失敗保留未知。其他非 macOS 平台的裝置名稱與電源清單尚未接上。

## 狀態與讀取

既有 `/api/state` 回應新增 `hardwareDetection`：

- `status`：not-started、running、complete、partial、cancelled、failed。
- `workers`、`completed`、`total`、`durationMS`。
- `results`：每項 key、狀態、耗時及平台原始能力證據。

`complete` 表示偵測工作完成並回傳結果，**不代表所有硬體能力支援或驗證通過**。消費端必須查看每項資料的建立／編解碼結果與硬體狀態。

狀態讀取只複製記憶體快照，不等待子程序。既有 `hardwareJPEG` 欄位改用非阻塞的 JPEG 快取讀取；尚未完成時不宣告可用，舊同步查詢介面仍保留給原有非 UI 使用者。

結果僅保留本次程序記憶體，不落地硬體快取；新視窗啟動不依賴前次結果。APP 在結果完成後透過 Host 與 Viewer 私有控制管道送出受限策略。Host 於新連線通道建立後固定採用；Viewer 格式公告可讀取晚到的解碼結果，原生解碼參數則於新解碼工作階段建立時取得快照。僅實測尺寸可用於略過重複探測或排除失敗組合；未確認／未測尺寸維持原本 runtime 檢查。硬解探測失敗不直接否定軟解，沒有用首張時間排列 codec 快慢。詳見 STREAM-OPTIMIZATION.md。

### 主畫面最佳化提示

前端完成主畫面繪製後，若偵測仍為 not-started／running，顯示帶旋轉動畫的「正在最佳化中」對話框；已完成則不顯示。完成、部分完成、失敗或取消都會自動關閉，不將原始錯誤顯示在對話框。

偵測期間沿用狀態輪詢，每 500 ms 可更新一次（請求／使用者操作仍有原有互斥保護），不採平常的 2.5 秒節流。服務失聯時也收起提示，避免無限旋轉。

使用者可按「繼續使用」或 Escape 收起，背景偵測不中止，本次頁面不再重複彈出。密碼／連線流程與其他對話框優先，不疊加最佳化提示。沿用淺色／深色樣式及減少動畫設定，提供繁中、英文、日文、韓文文案。

## 限制

這是啟動背景偵測，不是持續吞吐量 benchmark 或完整畫質驗收。未測 profile／level 全矩陣、4K、最小對齊、session 數上限、零複製、功耗及所有平台原生格式。

小型 detector 實驗在 M4 Pro 暖態的 4 路完整驗證約 0.87–0.94 秒；正式 APP helper 包含整個 Client 的載入／初始化成本，不能沿用該數字保證一秒內完成。無論耗時多少，UI 不等待結果。

初次 detector 整合限於編譯／語法檢查；後續依使用者要求執行的最佳化 smoke test 記錄於 STREAM-OPTIMIZATION.md。原始實驗 `try/` 仍由 `.gitignore` 排除，不作為產品建置依賴。

已通過 macOS ARM64 Client、Windows x64 Client（CGO）及 Linux x64／ARM64 detector 套件（無 CGO）編譯。未執行正式 APP 的啟動效能或 Windows 實機驗證。

## 手動深度測試

硬體分析右上角提供「深度測試」，滑鼠、觸控或鍵盤空白鍵／Enter 持續按住一秒後啟動；提前放開、離開按鈕、失焦或關閉視窗會取消長按。啟動後顯示進度條、百分比與已完成／總項目數，每 500 ms 輪詢更新。

每次重新執行平台完整清單（macOS CGO 共 19 項，Windows CGO 共 31 項，其餘建置共 7 項），沿用每項逾時與 4 路執行機制；單項失敗仍繼續其餘項目。啟動偵測或深度測試執行中禁止重複啟動，結束後隱藏進度條、進度文字與完成狀態／耗時，保留分析結果且可再次執行。深度測試結果不覆寫啟動時的連線策略。

編解碼表格將實測失敗或缺少成功證據的結果保守顯示為「不支援」；成功但加速方式未知仍顯示「可用 (加速未知)」。此為顯示政策，原始偵測證據不變。Windows CPU／GPU 查詢已通過 Windows amd64 無 CGO 交叉編譯，尚待 Windows 實機驗證。

## Windows 原生編解碼與格式矩陣

Windows CGO 建置新增 Media Foundation 直接測試：JPEG（MJPG）／H.264／HEVC／AV1 × BGRA（ARGB32）／NV12 全範圍／NV12 影像範圍 × 128×128／1920×1080，共 24 項；加 8 項正式 RGBA 路徑、8 項軟體獨立測試及 inventory，共 41 項。啟動與深度測試皆使用同一平台清單。

原生測試直接產生對應格式的灰階漸層，設定 nominal range（全範圍 0–255、影像範圍 16–235），不經 RGBA 包裝轉換。硬體 MFT 逐顯示卡選取；支援 D3D11 時提交對應原生格式的 GPU texture，否則提交 CPU 緩衝。候選必須實際產出影格才算成功；硬體候選失敗後嘗試系統軟體 MFT。

解碼使用當次編碼產生的影格與 sequence header，要求對應 BGRA／NV12 輸出，驗證尺寸、子格式及可讀緩衝。解碼另嘗試系統 D3D11-aware MFT；這類成功保留加速未知，只有硬體類別的 MFT 成功才標示硬體。CPU 軟體候選成功標示軟體運算。編碼失敗而沒有測試影格時，decoderAttempted=false，不偽造解碼實測結果。

回報 encode_status／decode_status、encodeOK／decodeOK、實際 MFT 名稱與 nominal range。range 欄位是協商屬性，不代表已驗證色彩精度；此測試不涵蓋全部 profile、位元深度或畫質。缺少 Windows codec／驅動時保留失敗證據，不自動安裝套件。原始資料以 probeKind=windows-native 區別，避免尚未接入正式串流的原生能力改寫正式 RGBA 路徑策略。

依據：[Media Foundation nominal range](https://learn.microsoft.com/en-us/windows/win32/api/mfobjects/ne-mfobjects-mfnominalrange)、[D3D11 解碼介面](https://learn.microsoft.com/en-us/windows/win32/medfound/supporting-direct3d-11-video-decoding-in-media-foundation)。

### Direct3D 裝置能力

Windows CGO 的 inventory 逐一枚舉實體 DXGI 顯示卡，分別呼叫 D3D11CreateDevice／D3D12CreateDevice，回報每張卡的成功狀態、HRESULT 與功能層級，在編解碼分析顯示 D3D11／D3D12 可用性。排除 WARP 軟體裝置；D3D12 動態載入，缺少 DLL 不影響 D3D11 與 APP 載入。

D3D 裝置可用僅代表圖形 API 能力；D3D12 尚未作為影片編解碼執行後端。硬體／軟體編解碼判斷仍取自原生 MFT 實測。Windows amd64 CGO 探測測試執行檔已成功交叉編譯；本機 macOS detector 與策略 Smoke 通過，Windows 驅動及 D3D 裝置結果仍須 Windows 實機驗證。

編解碼分析的優先圖形 API 依實際裝置建立結果選擇 D3D12 → D3D11 → 不支援，同時回報各顯示卡的 preferredGraphicsAPI；只要任一實體 GPU 支援 D3D12，整體優先值即為 D3D12。此優先值不改寫 MFT 實測證據，encoderGraphicsAPI／decoderGraphicsAPI 仍記錄實際採用的 D3D11 路徑。

只有 D3D12 明確不支援（無此 API／介面或 DXGI 不支援）且 D3D11 裝置建立成功，才退回 D3D11。裝置移除、記憶體不足等一般錯誤保留未知與 HRESULT，不以暫時失敗觸發降級；編解碼分析顯示「偵測未完成」。

優先 API 確定後，在該 API 實測可用的 GPU 中，以 FL 主版號、次版號的數值由大到小選擇；相同 FL 保留原枚舉順序，未知／無效 FL 不勝過已知值。編解碼分析的「優先圖形 API」顯示所選最高 FL，原始資料另保留 preferredGraphicsGPU。D3D11 建立裝置清單也改由 FL 12.1 向下排列，不再把查詢上限固定為 11.1；舊 runtime 回報無效列舉值時才縮短清單重試。

## 編碼優先與 Windows 軟解

協商依序選擇：本機可硬體編碼且接收端已確認硬解的格式，接著本機可硬體編碼且接收端可軟解／加速未知的格式；同級優先 HEVC，再 H.264。接收端新增 hardwareDecodeCodecs 公告欄位，並保持原本 Codecs 為所有可解碼格式，舊端未提供硬解欄位仍可協商。

Windows 正式接收路徑先使用 Media Foundation／D3D11 硬解，H.264／HEVC 轉為 Annex B，AV1 使用附長度的 low-overhead OBU。硬解建立或取回影格失敗才切換 FFmpeg CPU 工作階段；GOP 中途切換先要求 key frame，避免缺少參考影格。Windows 使用 FFmpeg 8.1.1 的 H.264／HEVC decoder，以及 libaom 3.13.1 AV1 decoder，不依賴 Windows HEVC／AV1 codec extension。macOS AV1 硬解採 VideoToolbox，軟體編解碼採 libaom，兩方向分開測試。

Windows AV1 已接入原生 MFT 編碼、OBU 封包、能力公告及協商。只在實際編碼成功後提供 AV1 硬體選項；同硬解等級內保留 HEVC、H.264、AV1 順序，避免只因新格式就改變既有預設。指定 AV1 時仍要求接收端公告支援，舊版不會收到 AV1。

## 獨立軟體測試

Windows CGO 包含 JPEG／H.264／HEVC／AV1 各 128×128、1920×1080 十六個 software 工作（各自編碼、解碼）。前端以「軟體獨立測試」另列，即使硬解成功仍執行。H.264／HEVC 軟編仍測試系統 MFT；AV1 軟編改用 FFmpeg／libaom，與軟解各自實測。三種影片的解碼使用內嵌獨立影格強制 FFmpeg CPU，與編碼測試結果分開。JPEG 沿用既有軟體編解碼。

每項依方向回報 encodeOK 或 decodeOK、實際 backend；解碼工作另回報 decoderSource=independent-fixture。CPU 解碼成功證據可補足未執行解碼的正式路徑，不能覆蓋已驗證的硬解。仍沿用每項 8 秒、最多 4 路的 helper 管理。

## Windows FFmpeg LGPL 建置

正式 Windows Client／Viewer 啟用 `ffmpeg,turbojpeg` build tags。`scripts/ffmpeg.py` 驗證固定來源 SHA-256，建置包含 H.264／HEVC／libaom AV1 解碼器、libaom AV1 編碼器、必要 parser 及色彩轉換的 FFmpeg 動態庫；明確檢查 GPL=0、nonfree=0。不啟動 ffmpeg.exe，不每張重建 decoder。libaom 使用內建 Win32 執行緒，不額外依賴 winpthreads DLL。

安裝包包含 `avcodec-62.dll`、`avutil-60.dll`、`swscale-9.dll`，以及 LGPL/BSD 授權、完整來源封存和重建腳本。DLL 可替換。macOS 同樣加入 FFmpeg 的 AV1 編解碼，硬解使用其 VideoToolbox 後端。一般未帶 `ffmpeg` tag 的開發建置會明確回報未包含此 decoder，不能當作正式 Windows 支援測試。

已在本機執行相同來源 CPU 函式庫的 H.264／HEVC／AV1 固定影格、尺寸、黑色像素、I/P/P 參考影格及損毀輸入 Smoke。Windows x64 Client／Viewer，以及 x64／ARM64 原生測試已交叉編譯，DLL 打包通過；Windows 實機的驅動與 GPU→CPU 切換仍待實測，不將交叉編譯當成實機通過。

### Windows 啟動缺檔修正（2026-09-13）

正式 MinGW GCC 建置的 `avutil-60.dll` 曾透過 `clock_gettime64`／`nanosleep64` 引入未隨附的 `libwinpthread-1.dll`，使桌面程序在顯示視窗前退出。僅檢查另一套 LLVM 工具鏈的建置結果不足以驗證正式包。

現在 Windows FFmpeg 明確選擇 Win32 執行緒，並關閉 POSIX clock_gettime／nanosleep／usleep 探測結果，使用 FFmpeg 既有 Windows 時間與 Sleep 路徑。建置快取加入來源版本、組態修訂與環境旗標；舊快取不再命中。`scripts/windows_runtime.py` 在函式庫快取讀取、DLL 複製和安裝包封裝時驗證 PE import 相依閉包及架構，缺少非系統 DLL 就停止打包。

登入前服務複製主程式時，也會複製這個建置所需的 FFmpeg DLL 到受保護服務目錄。Windows 啟動器新增啟動失敗對話框與結束碼，不再只把錯誤輸出到隱藏的 stderr。

## Windows 編碼與解碼分離（2026-09-13）

Windows 全部正式 RGBA、原生 BGRA／NV12 及軟體項目都拆成 `/encode` 與 `/decode` 兩個 helper，共 81 個工作（含一個硬體概況）。每個方向獨立排程、結果及 8 秒逾時；編碼錯誤或逾時不會跳過解碼。macOS 同樣拆分，詳見下節。

解碼只讀取預先內嵌的 JPEG、H.264、HEVC、AV1 壓縮樣本。正式 RGBA 解碼走接收端的硬解優先與 FFmpeg 軟解備援；原生格式矩陣只測 Media Foundation 指定輸出格式；軟體項目強制 CPU。原生格式不支援不會否定正式 RGBA 或獨立軟解成功證據。

介面以編碼、解碼兩張表分別顯示結果、後端與各自 helper 耗時（含程序啟動），解碼格式欄表示輸出格式。策略只採用該方向的證據，軟解通過也不會宣告有硬體編碼能力。

## macOS 編碼與解碼分離（2026-09-13）

macOS 統一採用 `/encode`、`/decode` 獨立 helper；JPEG、H.264、HEVC × BGRA、NV12-full、NV12-video × 兩種尺寸，各自測兩個方向，加上硬體概況，共 45 項（含新增的 AV1 硬體／軟體各兩種尺寸、兩個方向，共 8 項）。沿用每項 8 秒、最多 4 路的排程與兩張結果表。

解碼由內嵌 JPEG 或 H.264／HEVC 參數集與壓縮影格直接建立 CoreMedia sample，不建立編碼器，也不依賴編碼結果。保留 VideoToolbox 硬體要求，未加入 FFmpeg。解碼格式欄現在確實驗證該輸出格式及尺寸；原流程所有格式都解成 BGRA，因此部分 NV12 結果可能與舊版不同。各方向只回報自己的狀態、後端與耗時。
