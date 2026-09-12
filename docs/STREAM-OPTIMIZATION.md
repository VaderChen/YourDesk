# 偵測驅動影像路徑最佳化

## 已接上正式流程

1. **能力策略**：detector 完成後，APP 經既有私有 stdin 管道把小型策略送給常駐 Host。Host 在通道配對後取快照；既有連線不被新的偵測結果改動。不等待偵測才開始接受連線。
2. **尺寸精確比對**：已確認的 H.264／HEVC BGRA 路徑可省略重複的能力探測；明確編碼失敗的同 codec／尺寸組合可跳過。未知、逾時與未測尺寸仍走原本 runtime 驗證，並保留對端 codec 協商及使用者指定模式。
3. **JPEG 差分**：CPU 有適用 SIMD 能力時，允許最多 64 MiB 的前一畫面快照（已知低於 4 GiB 記憶體的 Mac 降為 16 MiB），逐列完整比較像素。超出預算或沒有策略時改用完整像素雜湊，不再隔點抽樣，避免漏掉單像素變化。
4. **區塊免複製**：RGBA 更新區域使用零起點、保留 stride 的只讀 view 交給同步編碼器；非 RGBA 保留通用轉換。支援非零來源起點。所有區塊編碼成功後才提交差分基準，失敗重試不遺失更新。
5. **macOS H.264／HEVC**：偶數尺寸 RGBA 直接讀取來源，非 RGBA 或奇數尺寸重用補邊緩衝區；右／下邊緣延伸最後一列或像素。CVPixelBuffer 跨影格重用，尺寸改變或關閉時釋放，CompleteFrames 完成後才覆寫。
6. **TurboJPEG 編碼／解碼**：正式軟體 JPEG 工廠優先使用 TurboJPEG，保留 Go JPEG 備援。編碼器重用 handle，RGBA 直接按 stride 讀取；解碼每張輸出獨立 RGBA，避免覆寫顯示端仍持有的資料。
7. **觀看端解碼策略**：APP 將同一台機器的解碼證據經 Viewer 私有 stdin 管道傳送一次；偵測未完成時不阻塞連線。一般觀看視窗與背景串流診斷共用非阻塞的格式公告，每秒可採用新抵達的策略；使用者指定軟體 JPEG 時不公告影片 codec。密碼、MCP 與最佳化控制訊息分流，策略不送給遠端裝置。

### 解碼策略的邊界與回退

- `decoders` 與 `encoders` 分開驗證；本機編碼失敗而沒有執行解碼，不代表不能觀看該 codec。逾時、缺少欄位或硬體屬性未知，不自行推定硬解可用。
- 已確認 128×128 解碼成功的 H.264／HEVC 可略過重複的格式能力探測。這只用於原有 codec 協商，不保證其他尺寸、profile 或 level；未知時使用既有背景能力快取，未完成時先以 JPEG 接收。
- 每個影片解碼工作階段建立時取得本機策略快照；晚到的策略不強制重建正在解碼參考影格的 session，於下一次建立時生效。
- macOS 以壓縮參數集的真實尺寸比對策略：已確認硬解時啟用硬解偏好；要求硬解的實測失敗時先嘗試軟解；未測尺寸仍由 VideoToolbox 自選。指定偏好的 session 建立失敗時，再嘗試原有自選路徑；實際執行模式仍從 session 查詢，不把偏好當成成功結果。
- Windows 正式接收端使用 Media Foundation 硬解與 FFmpeg H.264／HEVC／AV1 軟解備援，已確認成功可省略重複格式探測；同一實際尺寸的完整解碼失敗仍須保留獨立軟解成功證據。一般影片解碼失敗仍依原有協商回到 JPEG；參考影格遺失仍要求關鍵影格恢復。
- TurboJPEG 的 SIMD、IDCT 與色彩轉換由函式庫處理；仍固定準確 DCT／RGBA 輸出，不因 detector 改用較低畫質或假稱 NV12／GPU 零複製。

CPU SIMD 由 libjpeg-turbo 自身依 CPU 能力選擇，不強制所有 x64 執行 AVX2，也不把 CPU 加速誤標為 GPU／專用硬體 JPEG。硬體後端仍需自己的建立／執行驗證。

## 後續最佳化：重複探測、接收端複製、靜止畫面與通道轉換

- 主畫面的影片能力查詢等待背景 detector 結束後才啟動補充探測，並先套用已確認的本機策略；未知／取消／失敗仍保留原有背景回退，不等待結果才開啟 APP。
- 解碼能力快取獨立於編碼能力：Viewer 的格式公告不再連帶啟動本機影片編碼器。完整能力查詢與解碼查詢共用同一份解碼快取；已驗證的同 codec／128×128 能力不重測。不跨尺寸推論，不把硬解失敗當成軟解也失敗。
- 完整、零起點且緊密排列的獨立 RGBA 解碼結果，直接交給合成畫面持有，省去第二張完整影像配置／複製。區塊影像、非 RGBA、非零起點或不同 stride 保留原有合成路徑；解碼器仍須每張輸出獨立記憶體，不自行重用顯示端仍持有的緩衝。
- 重送的完整 RGBA 與現有畫面逐位元組相同時，保留原畫面，不因這張重送觸發紋理上傳、超解析度提交或補幀。不同螢幕仍強制顯示更新；輸入、控制、視窗與必要繪圖流程不停止。未啟用補幀時不在每次繪圖解析補幀後端能力。
- Mac VideoToolbox 編碼／解碼與 Windows DXGI／GDI 擷取共用紅藍通道交換：ARM64 使用 NEON、支援 SSE2 的 x64 使用 SSE2；剩餘像素及其他架構走純量，無 CGO 的 GDI 使用 Go 回退。alpha 維持 255，支援列 padding 與未對齊記憶體；不是色域／YUV 矩陣變更，不改畫質或 JPEG sampling。
- 本輪僅做編譯檢查；新增像素轉換與合成邊界案例供後續驗證，未執行測試或效能 benchmark，不宣稱實測加速比例。尚未部署、發布；SYSTEM MCP 權限待辦未於本輪處理。

本輪 macOS ARM64、Windows x64／ARM64 Client 與 Viewer（靜態 TurboJPEG）編譯通過；Linux x64／ARM64 無 CGO 共用套件編譯通過。像素轉換及 Viewer 測試程式僅編譯，未執行。影片探測合併不包含用途／尺寸不同的 JPEG 與超解析度探測；未改成整個視窗停止繪圖，也未移除解碼器內部所有複製。

## TurboJPEG 建置與打包

- 固定 libjpeg-turbo 3.2.0，下載官方來源封存並驗證 SHA-256。
- 原始碼與靜態函式庫置於被忽略的 `.local-run/turbojpeg/`，不依賴 `try`。
- CMake 建立對應架構的靜態庫；x64 SIMD 需要 NASM。此次本機已安裝 NASM 3.02。
- `scripts/release.py` 的 macOS／Windows Client 與 Viewer 自動使用 `turbojpeg` build tag 並靜態連結，無須目標機另裝 TurboJPEG DLL／dylib。
- `runUITest.command` 亦改用同一建置入口，避免開發版默默退回 Go JPEG。
- 未帶 tag 或 CGO 關閉的建置維持 Go JPEG。現有 Linux CLI 與 WinPE 發布路徑維持無 CGO，不宣稱已啟用 TurboJPEG。
- 發布流程將 `LICENSE.md`、`README.ijg` 放入 `ThirdPartyLicenses/libjpeg-turbo`，macOS 放入 App Resources，Windows 安裝程式沿用既有 ThirdPartyLicenses 收錄。

本機編譯或測試範例（不發布）：

```sh
python3 scripts/turbojpeg.py darwin/arm64 go build -o try/detector/yourdesk-client-optimized ./cmd/client
python3 scripts/turbojpeg.py darwin/arm64 go test ./internal/desktop ./internal/video ./internal/optimization ./internal/hardwareprobe -run 'TestDelta|TestPatch|TestTurbo|TestSoftwareJPEG|TestFallback|TestReceiveDouble|TestPolicy|TestDetectorPolicy|TestBackgroundDetectorSmoke' -count=1 -timeout=60s
```

Windows 交叉編譯由 release.environment 選取相應 MinGW／LLVM-MinGW 編譯器；未執行打包、簽署、上傳或 Release。

## Smoke test

使用合成影像、本機測試子程序，沒有連線遠端站台或擷取真實桌面。

- 策略驗證、快照不可被呼叫端修改、尺寸不可外推。
- 偵測 pending／未知／逾時不誤判；明確失敗可被排除。
- 4 路真實 detector 子程序完成 19 項，產出策略。
- 完整比較與完整雜湊皆抓到奇數座標單像素修改；非零來源起點、區塊 stride、編碼失敗後重試。
- TurboJPEG 正式工廠選擇、Go 解碼相容、129×129 非對齊子圖、獨立解碼輸出、毀損輸入拒絕及 CPU 後端失敗回退。
- JPEG／H.264／HEVC 各 24 張合成影格，串行與雙緩衝解碼像素雜湊一致；兩種影片 codec 各含 21 張參考影格。

上述 smoke test 已通過。正式 macOS Client 的 helper 亦完成 H.264 BGRA 128×128 硬體編解碼。macOS Client／Viewer、Windows x64 Client／Viewer、Windows ARM64 Client 靜態 TurboJPEG 建置通過；Linux x64／ARM64 無 CGO 共用套件編譯通過。Windows 尚無實機 smoke test，未執行完整安裝／簽署／發布驗證。

後續接入 Viewer 解碼策略這一輪，依要求僅進行編譯檢查：macOS ARM64、Windows x64／ARM64 的 Client 與 Viewer（靜態 TurboJPEG）建置通過；新增的解碼證據判讀與策略驗證測試案例僅編譯，未執行。前段 smoke test 是接入前的紀錄，不代表已驗證本次新增的解碼策略或實機連線。

## 尚未做的事

首張建立／編解碼時間不是持續效能 benchmark，因此不依它改寫 HEVC／H.264 排序，也不以不同畫質的輸出大小挑最快 codec。尚未動態切換 NV12 輸入、實作 capture → encoder 零複製、廣色域管理或套用自訂 YCbCr SIMD 矩陣；這些需要另外驗證格式與品質。

策略目前送給 APP 自己啟動的 Host 與 Viewer；登入前系統服務、獨立 CLI Host，或未收到父程序策略的獨立 Viewer，保留既有能力探測。Viewer 不再啟動另一組完整 detector，以免每個觀看視窗重複占用硬體資源。TurboJPEG 與免複製等共用編解碼改善仍依其建置配置生效。

This software is based in part on the work of the Independent JPEG Group.

## 分析結果驅動配置排序（2026-09-13）

「串流方式」在編碼選單下方新增傳輸策略：均衡（預設）、低延遲、省頻寬。移除原本兩個自動配置按鈕。偏好會保存到設定，傳入一般 Host 與 macOS／Windows 未登入服務。變更會沿用現有 Host 設定重啟流程；啟用未登入服務時沿用既有設定保護。

候選先經過正式串流編碼器、對端解碼公告、相同實際尺寸的編碼證據及本工作階段失敗紀錄篩選。尺寸按正式影片編碼補為偶數，不能用原生格式矩陣的成功結果代替正式 RGBA 路徑。未知尺寸仍需後端實際建立與編碼成功。

排序按以下優先序逐層比較，不以加權分數抵消硬體偏好：

1. 雙端硬體、編碼端硬體、解碼端硬體、全軟體。
2. 同級優先採用目前尺寸已實測成功的配置。
3. 同級且證據相同：均衡為 HEVC → H.264 → AV1；低延遲為 H.264 → HEVC → AV1；省頻寬為 AV1 → HEVC → H.264。

這些是用途偏好，不是持續效能測量結果；不拿 helper 啟動耗時估計 FPS，不依 D3D 版號或 SIMD 清單推論某個 codec 可用。對端硬解公告目前仍是格式能力，並不保證目前尺寸。原有 FPS、碼率、GOP 上限照使用者設定，不因選擇偏好擅自改值。

正式 Host 目前提供硬體影片及 JPEG 路徑，因此只將實際可建立的路徑納入；不將已知的軟解能力誤列為軟編。影片候選耗盡時使用既有 JPEG 回退；使用者明確指定格式時不另選其他影片格式。

失敗紀錄改成 codec＋尺寸：某個編碼器建立或壓縮失敗，本張先退回 JPEG，下一張嘗試排序中的下一候選；不再因單一硬編失敗就停用所有影片硬編。既有連線策略仍採連線開始時的分析快照。

已加入排序模式、硬體優先、尺寸證據、未知對端、明確指定格式及單一失敗回退測試；macOS Smoke 通過。Windows 僅交叉編譯，尚未驗證實機連線效能。
