# Android 深度檢查紀錄

本輪為檢查，不修改 Android／桌面功能，不重建或替換 AAR，不安裝 APK，不操作手機，不同步 GitHub。隔離測試位於系統暫存目錄；此文件是本輪唯一新增的專案檔案。

## 結論與範圍

確認 17 項可採取修正的問題：5 項 P1、12 項 P2。優先處理記憶體上限、解碼協商／初始化、Shell 輸入、16 KB 相容性，再處理恢復／生命週期及設定一致性。

檢查涵蓋：Java Activity／WebView bridge、JPEG／MediaCodec、Surface、Go core／Pion／分片、signaling／認證、Shell、相機／QR、憑證儲存、前端事件、Gradle 與實際 AAR。不能用桌面版測試通過替代 Android 驗證。

- 原始碼位於 `android/`，包含獨立的 `android/core` Go module。後續修正時核對 Git 索引：既有 65 個檔案其實已追蹤，但原 `/android/` ignore 規則會隱藏新增檔案；不能僅從 ignore 規則推論既有檔案未追蹤。
- APP 設定為 `0.26.0915 build 1801`，minSdk 26、targetSdk／compileSdk 35，ABI 僅 arm64-v8a。
- Gradle 直接使用 `android/libs/androidcore.aar`。從其中 `libgojni.so` 的 build info 確認：Go 1.27.0、Pion WebRTC v4.1.6、SCTP v1.8.40。桌面 module 的 Pion 為 v4.2.20；本輪按 Android 實際副本／產物分析，未假設它使用桌面修正或最新版。
- 證據標記：「重現」是隔離測試觀察到目前缺陷；「契約」是原始碼與官方 API 要求直接衝突；「靜態」是具體可達控制流程，但未在手機實測。

## P1：優先修正

### A01 — 影格佇列的 512 張上限會失效〔重現〕

位置：`android/core/api.go:138–154`。

佇列滿時倒找 keyframe；若最近 keyframe 位於索引 0，`drop=0`，接著仍 append。實測首張 keyframe 加 2,047 張差分，全部 2,048 張被保留，超過宣告的 512。沒有位元組預算；讀取落後時記憶體、找 keyframe 與搬移成本一同上升。若已無 keyframe，則直接淘汰相依差分且沒有要求恢復。

建議：同時限制張數與實際保留 bytes，溢位後標記基底失效、要求完整影格；enqueue 不等待解碼或大型序列化。不要只把 512 改小。

### A02 — 實際 AAR 的 Go 原生庫不符合 16 KB ELF 對齊〔產物檢查〕

位置：`android/app/build.gradle.kts:32` 引入的 `android/libs/androidcore.aar`。

直接讀取 AAR 內 arm64 `libgojni.so` 的 ELF program headers：四個 PT_LOAD 全為 `p_align=4096`；其中部分 file offset 與 virtual address 在 16 KB 下亦不同餘。這不是只有 APK ZIP 對齊的問題，需重新連結原生庫。原生 16 KB 環境不能據此產物宣稱受支援；某些系統可能有相容模式，未實測特定手機啟動失敗。

建議：以支援 16 KB 的 NDK／連結設定重建 Go AAR，核對所有依賴 `.so`，再檢查 APK zipalign 與 16 KB 裝置。不能只升級 Gradle 或重新壓縮 APK。[Android 官方對齊與預編譯庫要求](https://developer.android.com/guide/practices/page-sizes)。

### A03 — H.264／HEVC 的 CSD 初始化格式錯誤〔契約＋實際資料〕

位置：`video/MediaCodecVideoDecoder.java:79–81`、`video/EncodedVideoFrame.java:58–59`，均位於 `android/app/src/main/java/com/yourdesk/android/`。

Parser 保存原始 NAL，configure 直接放入 csd-*，沒有起始碼；HEVC 還把 VPS／SPS／PPS 分散成 csd-0／1／2。官方要求參數集帶 `00 00 00 01`；HEVC 的三個參數集應合併在 csd-0。現有 `fullscreen-h264-1080.bin` 測試資料的 SPS 開頭為 `67641028acb80f00`、PPS 開頭為 `68ee0f2c8b`，確實不是帶起始碼的 CSD。

建議：依 codec 建立正確 CSD，增加實際 configure／解碼測試。部分 decoder 可能容忍或靠 access unit 內的參數集成功，不能因此認定跨廠牌正確。[MediaCodec CSD 契約](https://developer.android.com/reference/android/media/MediaCodec#CSD)。

### A04 — 公告不可播放的 codec，失敗後不會降級 JPEG〔靜態〕

位置：`video/DecoderSupport.java:25–26`、`video/MediaCodecVideoDecoder.java:76–77`、`MainActivity.java:637–659,1170`。

能力公告包含「僅有 software decoder」的 H.264／HEVC，但實際播放只找 hardware decoder。即使找到硬體候選，也未驗證當前尺寸／profile。失敗路徑只要求 `video.keyframe`，沒有撤回能力或改走軟解／JPEG；能力公告只在連線時發一次。Host 不會因收到 IDR 請求就知道 Android 無法播放該 codec。

建議：只公告真正可用的路徑；配置失敗有有限次候選嘗試，之後撤回該 codec、重新協商 JPEG。`FfmpegRuntime` 目前只讀版本，不是實際軟解備援。

### A05 — Shell 輸入框吞掉 Enter／Backspace／Ctrl+C〔瀏覽器重現〕

位置：`android/app/src/main/assets/terminal.js:3–5,18,29`。

程式主動將焦點移到單行 password `input-proxy`；只監聽 input 並送非空字串，每次立即清空，未處理特殊鍵。使用現有 HTML／JS／xterm，在獨立 Edge headless profile 中透過 CDP 實際輸入：

```text
輸入 a                         → 送出 ["a"]
再按 Enter、Backspace、Ctrl+C   → 仍只有 ["a"]
改由 term.focus() 後按同三鍵    → 新增 ["\r", "\x7f", "\x03"]
```

所以並非鍵盤測試未送出事件，而是正式聚焦路徑沒有轉送；Shell 無法正常提交命令或刪字。這是桌面 Chromium 上的前端重現，不等於各 Android IME 已實測。

建議：統一 xterm／IME 的輸入來源，處理控制鍵、刪除及 composition，並避免重複送字；補 Gboard、中文 IME、實體鍵盤測試。

## P2：穩定性、恢復與功能正確性

### A06 — 分片大小／配置預算／已完成序號驗證不足〔重現〕

位置：`android/core/internal/p2p/frame.go:48–61`。

未限制分片 payload 大小，以 `total*28KiB` 預配結果，而非真實資料長度。實測：1-byte payload 保留 28,672 bytes backing array；1,200 個空分片、共 52,800 wire bytes，形成 len=0、cap=34,406,400 的影格；28,673-byte 超長分片及完成序號重放皆被接受。

這是已認證 Host 可觸發的記憶體放大，不是未認證遠端漏洞。配合 A01 更容易耗盡記憶體。桌面 assembler 已有相關保護，Android 副本沒有。應補片段／總 bytes 上限、拒絕空結果、按實際長度配置、完成序號去重。

### A07 — 取得 input buffer 後直接返回，會耗盡 decoder 輸入槽〔契約〕

位置：`video/MediaCodecVideoDecoder.java:36–39`。

`dequeueInputBuffer` 已交付 buffer index；容量不足就回 -1，既不 queue 回去，也不 reset／release。後續大 keyframe 再次走此路徑會持續扣住輸入槽，最後沒有 buffer 可取。應在此失敗路徑回收／重建 decoder，並處理最大輸入尺寸。[MediaCodec buffer 所有權](https://developer.android.com/reference/android/media/MediaCodec#DataProcessing)。

### A08 — JPEG 缺幀後仍繼續使用失效基底〔靜態〕

位置：`MainActivity.java:815–818,839`。

明確序列：已顯示 seq10 → seq11 遺失 → seq12 被丟棄，但 composedSequence 更新到 12、bitmap 不變 → seq13 通過連號檢查，直接貼到 seq10。遺失區域持續錯誤，與註解「等下一個完整 keyframe」不符。

應將基底標記失效並要求完整影格，直到完整畫面套用才恢復接受差分；不能只把序號往前推。

### A09 — JPEG 先完整配置，再驗證實際尺寸〔靜態＋契約〕

位置：`MainActivity.java:778–780,821–825,861–864`。

外層尺寸限制不等於 JPEG 真正尺寸。宣告 1×1、內含 8192×8192 JPEG，程式先嘗試配置約 256 MiB 的 ARGB bitmap，之後才判定超界。`catch(Exception)` 也不攔截 OutOfMemoryError。是否 OOM 依裝置而異，本輪未對裝置施加耗盡記憶體測試。

應用 `inJustDecodeBounds` 預檢實際 dimensions／像素數／範圍，再配置。[Android 大型 Bitmap 解碼建議](https://developer.android.com/topic/performance/graphics/load-bitmap)。

### A10 — IPv4 與暫時失敗被 process-global sync.Once 永久快取〔靜態〕

位置：`android/core/third_party/anet/anet_android.go:14–18,38`。

首次沒有路由，錯誤永遠保留；恢復網路後重新 Connect／NewViewer 仍取得同一錯誤。首次取得 IP 後換 Wi-Fi／行動網路，LAN direct 還會使用舊 IP。網際網路連線另有 wildcard STUN 路徑，因此不能推論所有換網一律失敗。

應按連線或 Android 網路變更事件刷新介面，且不永久快取暫時錯誤；測試離線啟動、換 AP、Wi-Fi→行動網路及 IPv6-only。尚未手機換網實測。

### A11 — Close 與 Connect 註冊 cancel 之間有取消缺口〔重現〕

位置：`android/core/api.go:48–56`。

Connect 先 Close，再另外註冊新 cancel；外部 Close 可以落在兩者之間。隔離測試透過現有 state handler 加 barrier，讓中止 Close 完成後放行 Connect，仍觀察到新 TCP/TLS 連線；再 Close 一次才取消。Java generation 阻擋舊結果上畫面，但舊 Connect 仍可能占住唯一 io executor、拖延下一個連線。

建議以同鎖與連線世代管理完整的建立／取消過程，而非僅檢查 UI 回傳結果。

### A12 — 桌面 IME 提交文字沒有轉送〔瀏覽器重現〕

位置：`android/app/src/main/assets/desktop.js:21–22`、`MainActivity.java:349`。

Native 聚焦 keyboard-proxy，但前端只處理 keydown／keyup，沒有 input／composition。CDP 插入「中文」後 input 內有文字，控制訊息仍為空。IME committed text 不保證逐字有一般 keydown；即使 Enter 可送，也不能代表文字輸入正確。

建議建立文字輸入事件路徑，分清實體鍵與 IME 提交文字，避免重複送出。

### A13 — QR／站台的自訂 signaling 位址被忽略〔瀏覽器重現＋靜態〕

位置：`android/app/src/main/assets/connection.js:40`、`MainActivity.java:1166`。

站台會保存 signal，但連線 payload 只有 mode／room／secret；native 再硬編碼公用 signaling URL。測試讀入帶自訂 signal 的站台，送出的 connection.signal 為空。自架伺服器的房間因此連錯位置或失敗；直接 IP 模式另由 core 處理，不在此結論內。

建議沿用選取站台的 signal，驗證 scheme／host／TLS，再傳給 core；未設定時才使用預設。

### A14 — 從 JS 背景執行緒呼叫 CameraX unbindAll〔契約〕

位置：`MainActivity.java:1014`、`android/app/src/main/assets/app.js:44`。

已開始掃描後，按取消或切回手動會透過 JS bridge 執行 stopQrScanner，直接變更 View、呼叫 unbindAll。bridge 是背景執行緒；CameraX 要求主執行緒，否則拋 IllegalStateException，導致後面的 provider／scanner 清理中斷。應將整個啟停流程序列化至 UI thread。

依據：[JS bridge 執行緒](https://developer.android.com/develop/ui/views/layout/webapps/native-api-access-jsbridge)、[CameraX unbindAll](https://developer.android.com/reference/androidx/camera/lifecycle/ProcessCameraProvider#unbindAll())。

### A15 — 相機生命週期與 Activity 不一致，取消後仍可能重新綁定〔靜態〕

位置：`MainActivity.java:974–980,1234–1244`。

Analyzer 捕捉 Activity，卻綁 ProcessLifecycleOwner；onDestroy 不解除相機或 close scanner。Activity 因 uiMode 等設定重建後，舊 analyzer 可保留舊 Activity/WebView。此外，CameraProvider Future 完成時不檢查取消／銷毀／世代，使用者取消初始化後仍會綁定相機。

應綁 Activity lifecycle，在銷毀時清理，並拒絕過期非同步回呼。[CameraX lifecycle 清理責任](https://developer.android.com/media/camera/camerax/architecture)。

### A16 — 解析度選項等價，畫質與縮放設定互相覆蓋〔靜態〕

位置：`MainActivity.java:431–446`、桌面 `internal/streamconfig/config.go:84–89`。

75%、60%、50% 最後只傳同一個 imageEnhancement=true，沒有實際比例。Host 的 ComposeLegacy 將 true 全解讀成 50%。改縮放時傳 quality=null，把已有高畫質改回 standard；改畫質時傳 scale=null，又把縮放改回 100%，但 UI 選取值仍保留。

建議用完整選取狀態產生單一設定，傳明確 scalePercent，核對 Host ack 的 effective settings，而非把所有比例當成 boolean。

### A17 — Surface 重建後未要求 IDR，靜止桌面可能持續黑畫面〔靜態〕

位置：`MainActivity.java:218–234,630–645`、`video/MediaCodecVideoDecoder.java:29`。

若 pending 是非 keyframe，Surface destroy 將 decoder reset 成 awaitingKeyframe；available 只重送 pending，queue 回 0，present 只有負數才要求 IDR，因此沒有恢復要求。Host 的靜止補畫僅適用 JPEG；H.264／HEVC 無新擷取時不編碼，GOP 不會因時間流逝自行前進。

在持續擷取／DXGI 靜止路徑下，必須等桌面後續變動產生 IDR 或其他操作才恢復。Surface available／等待 IDR 應主動要求完整影格並節流重試。未宣稱手機實機重現。

## 效能與維護缺口（與確證缺陷分開）

- **JSON／Base64 跨語言影像管線成本高，還共享接收鎖。** `api.go:168–188` 持 frameMu 做 Marshal、Base64 與 string 複製；DataChannel enqueue 也拿同鎖。1 MiB 壓縮影格 benchmark（Apple M4／Darwin、20 次、非 race）為 **776,058 ns/op、3,293,608 B/op、9 allocs/op**，尚未計 JNI／Java JSON／Base64／NAL 複製與解碼。應縮短鎖範圍，使用有界二進位傳遞與獨立解碼 worker。此量測不是 Android ANR 實測。
- **主執行緒每輪最多解 32 筆，之後才等 50 ms。** `MainActivity:693–711` 的 frame pump 包含 JNI、JSON、JPEG 解碼與合成、MediaCodec 操作。需要手機上的慢解碼／背景恢復／4K 壓測；不能僅因網路回呼沒直接解碼就認為已無背壓。
- **FPS 不是實際顯示 FPS。** 每次 pump 只有 presented boolean，加一次 frame count，且沒把 drainOutput 的正數列入；顯示值最多約 20，不代表 decoder 真正吞吐量。
- **Go source 沒有與 AAR 建置建立依賴／新鮮度檢查。** Gradle 只引用 files(AAR)，未見 gomobile 建置任務；修改 core 不會自動進 APK。不能憑修改時間斷言現有 source 與 binary 不一致，但後續修復若只 assembleRelease，會繼續使用舊 core。需來源指紋、可重現 AAR 建置及依賴鎖定。
- **Android 的 Control queue 仍是桌面修正前副本。** `android/core/internal/p2p/peer.go:422–447` 在鎖外讀 ReadyState，flush 與新 send 分離，滿載淘汰已接受事件。應移植桌面 ordered sender 並補相同競態測試；本輪沒有另外做 Android 真實通道開啟競態重現。
- **原 core 沒有 Go 測試；沒有 Gradle wrapper。** 現有 androidTest 著重全螢幕／觸控，不能覆蓋這些失敗路徑。
- **首次相機授權仍待手機確認。** JS 同時請求權限並開始掃描，沒有授權結果 callback 重啟；若先因無權限失敗，允許後可能仍需重開掃描。本項未算入 17 項確證清單。

## 已執行與未執行的驗證

已完成：

1. Android 65 個檔案的範圍盤點；Android source／既有 AAR 未修改。
2. 四個自有 JS 檔案的 `node --check` 通過。
3. 獨立 Edge headless profile 載入真實 assets／xterm；mock 只替代 Native bridge。Shell 控制鍵、桌面 IME、signal 遺漏共 3 個缺陷案例重現。測後關閉該 browser instance。
4. Android Go core 的 `GOTOOLCHAIN=go1.27.1 go test -mod=readonly -race -vet=all -count=1 -timeout=90s ./...` 通過編譯／vet；**全部原套件為 [no test files]**，不能宣稱完整功能 race 已驗證。
5. `/tmp` overlay 注入 6 個 Go 缺陷重現測試：佇列超限、差分淘汰、微小影格配置、空分片放大、超長／重複分片、取消缺口；以 `-race -run TestReview` 執行通過。這些測試斷言目前錯誤行為存在，**不是修正後回歸通過**。
6. 直接檢查預編譯 AAR 的 Go build info 與 ELF PT_LOAD；不執行 Android native library。
7. 對照 Android 官方 MediaCodec、CameraX、WebView bridge 與 16 KB 要求。

環境限制：本機未找到可用 Java runtime／JDK、Android SDK、Gradle 或 adb。本輪沒有安裝工具，因此未執行 assembleRelease、lint、Android instrumentation、APK 簽章／zipalign、真機 codec、16 KB 開機、相機權限或換網測試。亦未執行完整依賴漏洞資料庫掃描；不宣稱沒有 CVE。

安全檢查未發現可證明的 WebView 注入／認證繞過：UI 使用本地 assets、外站導覽受阻、站台文字以 textContent 顯示；密碼採 AndroidKeyStore＋AES-GCM 且停用備份；signaling 有 room／role／HMAC／時間驗證，直連綁 TLS exporter。這不是完整安全保證。

目前範圍沒有找到 Android Host、音訊播放、原生剪貼簿／檔案傳輸、下載更新或外部檔案分享實作；視為未提供功能，不將桌面功能直接當作 Android 已實作。

## 建議修正與驗收順序

1. 建立可重現 Go AAR／APK 建置，補 16 KB ELF 與 source freshness gate。
2. 修 A01／A06 的有界接收，以及 A03／A04／A07 的真實解碼能力與降級。
3. 修 JPEG／Surface 恢復；用慢 decoder、缺幀、靜止桌面、背景／返回做壓測。
4. 修 Shell／桌面 IME；確認 Enter、Backspace、Ctrl+C、中文組字與大段貼上。
5. 修取消／換網／相機 lifecycle；核對設定與自架 signaling。
6. 至少在 API 26、主流新 Android、16 KB 環境及不同 codec 廠牌上驗收，再評估發布。完整保留目前桌面修改，不以同步 GitHub 代替驗證。
