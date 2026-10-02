# Android 建置、簽章與驗證

更新於 2026-10-02。Android Viewer 位於 `android/`，使用獨立 Go module；桌面版的編譯不能代替 Android core、APK 與手機驗證。

一般使用者可直接下載 [Android 1.26.1002 build 2301](https://github.com/VaderChen/YourDesk/releases/tag/1.26.1002-build-2301)；本次與桌面版合併發行，安裝方式與驗證範圍見[發行說明](RELEASE-1.26.1002-build-2301.md)。既有功能與歷史實機結果見 [Android 功能紀錄](ANDROID-RELEASE-2026-10-01.md)。以下為操作、建置與維護文件。

## 發行範圍

本版提供 Android 8.0 以上的 arm64 手機 APK，支援遠端桌面、右鍵／遠端捲動、多螢幕切換、遠端聲音、Shell、站台新增／編輯／刪除、加密記憶密碼及 QR 匯入。影像使用 JPEG 與 MediaCodec H.264／HEVC，聲音使用系統 Opus／AAC 解碼與 AudioTrack，並支援 PCM。剪貼簿同步尚未實作；AV1、獨立檔案傳輸與 FFmpeg 軟解碼不在本版範圍。相機為選用功能，沒有相機仍可手動新增站台。

- 版本由 `android/version.properties` 統一管理：`1.26.1002 build 2301`、versionCode `29849221`。每次正式更新必須遞增 versionCode。
- 正式套件為 `com.yourdesk.android`；Debug 使用 `com.yourdesk.android.debug`，實機測試不覆蓋正式資料。
- 站台與密碼更新使用一次儲存交易。密碼由 Android Keystore／AES-GCM 保護，依「信令伺服器＋遠端 ID」隔離；刪除站台會清除對應密碼。編輯時留白保留密碼，另有明確的清除選項。
- QR 僅接受 YourDesk v1 白名單欄位，拒絕密碼、重複欄位與不安全信令位址；掃描後必須確認再儲存。相機拒絕、取消、背景切換均能返回手動操作。
- WebView 只允許本機 assets；離開工作階段、背景返回、影像逾時及遠端斷線都有一致的清理與狀態回報。

舊版 `0.1.0` 曾使用 Debug 憑證簽署正式套件 ID，無法直接以不同憑證的 APK 覆蓋。若保有與原安裝一致的舊金鑰，Android 9 以上可使用 [apksigner 的簽章輪替](https://android.googlesource.com/platform/tools/apksig/+/master/src/apksigner/java/com/android/apksigner/help_sign.txt)：核對舊憑證、建立舊金鑰至正式金鑰的 lineage，使用 `--rotation-min-sdk-version 28` 簽署相同程式內容的遷移 APK，先以 `adb install -r` 完成遷移，再以 `adb install -r` 安裝正式交付 APK。保留 installed-data 能力，禁止舊簽章 rollback；遷移 APK 僅用於已有舊版的裝置，不替換正式發行成品。缺少原金鑰或裝置不支援時，應先備份站台、確認可重新取得密碼，並取得清除資料的同意後才卸載重裝；卸載會移除 App 資料及 Keystore 金鑰。建置腳本不會自動卸載 App，也不會更換發行金鑰。

## 桌面縮放與可視區操作

- 雙指開合：以兩指中心縮放畫面，範圍 100%～800%；100% 表示完整畫面適合手機顯示區。
- 雙指拖移：移動放大後的可視區；到達來源邊緣即停止，不會拖出額外空白。
- 工具列的百分比按鈕：可選放大、縮小、適合畫面，以及「單指拖移可視區」。開啟後按鈕會顯示「拖移」；關閉即可用單指操作遠端滑鼠。
- 預設單指點擊／拖曳操作遠端。雙指操作與抬起其中一指後的尾端事件保留在手機，不傳成遠端點擊。全螢幕仍支援雙指縮放與拖移。
- JPEG、H.264／HEVC 共用顯示與觸控轉換；旋轉或切換全螢幕保留倍率與可視中心，換遠端螢幕或重新連線則回到完整畫面。
- 每次開始串流時，關閉按鈕回到標題列右上角並垂直置中，位置避開系統列及瀏海；退出全螢幕仍對齊標題列。可拖曳至安全區內其他位置，旋轉時依比例保留位置，重新連線則回復預設。

「來源解析度」調整 Host 的傳輸影像尺寸；百分比按鈕調整手機上的檢視倍率。兩者可獨立使用。

## 右鍵、遠端捲動與螢幕切換

- 預設「單指操作遠端」模式下，短按為左鍵，拖曳為左鍵拖曳；在同一位置長按為右鍵。長按後抬指不再送左鍵，移動、取消、失焦或第二指加入會取消尚未觸發的長按。
- 點工具列百分比按鈕，選「單指捲動遠端內容」；上下滑動即捲動遠端視窗，向上滑動查看下方內容，向下滑動查看上方內容。捲動前先把遠端游標定位到起點，不改變本機可視區。再次點選目前勾選的捲動／拖移模式即可恢復單指點擊及長按右鍵。
- 「單指拖移可視區」與「單指捲動遠端內容」互斥；雙指仍只縮放／拖移本機可視區。進入全螢幕保留模式，退出全螢幕後可在工具列切換。
- 外接滑鼠左／右／中鍵、移動與垂直滾輪使用相同原生座標換算與螢幕邊界檢查。小於一格的滾輪量會累積，焦點丟失或取消時釋放按鍵。事件定義依據 [Android MotionEvent](https://developer.android.com/reference/android/view/MotionEvent)。
- 頂端「切換遠端螢幕」選單依 Host 的 `displays` 回報建立，選擇後送出帶序號的 `display-select`；僅有一個螢幕時不可切換，沒有可用螢幕時明確顯示狀態。
- 切換期間暫停輸入，收到相同要求的 Host 確認及新螢幕影像後才恢復；舊確認與舊螢幕影格不會覆蓋新選擇。H.264／HEVC 換螢幕重建解碼參考並等待關鍵影格，倍率回復 100%。輸入附上螢幕編號，Host 可拒絕落在其他螢幕的舊操作。
- 五秒內未收到切換確認會顯示逾時，可重試或重新連線；晚到的有效確認仍可恢復。螢幕移除、數量變更與重新連線會更新清單；不捏造未回報的螢幕。

## 遠端聲音

- 工具列的喇叭按鈕切換聲音，預設關閉並保存選擇；長按可查看協商、播放或失敗原因。音量由手機媒體音量鍵控制，全螢幕仍持續播放。
- 播放遠端電腦的系統輸出，不擷取手機麥克風，也不要求錄音權限。Host 必須支援 `audio.configure`；自動格式協商使用 `audio.capabilities`。
- 依兩端能力選擇「AAC 硬體來源 → Opus → AAC 軟體來源 → PCM」，來源或本機解碼器失敗時切換下一個共同格式。舊 Host 未提供能力查詢時，依序嘗試手機可解碼的 Opus／AAC／PCM；完全不支援聲音的 Host 會顯示原因，桌面操作仍可使用。
- 音訊品質跟隨現有畫質設定，Opus／AAC 的目標碼率與桌面端相同；PCM 固定 48 kHz、16-bit、雙聲道。音訊使用獨立 `audio-v1` DataChannel，接收最多保留八包，PCM 佇列也有上限；控制協商及非阻塞播放分開在背景工作者執行。
- 啟停、品質變更、背景返回與重連均使用新的世代；舊世代、重複、倒序及無效封包會丟棄，缺包會清除解碼與播放歷史，避免補播陳舊聲音。每秒更新 Host 租約；通知遺失時 Host 六秒後停止擷取。
- App 進入背景或失去音訊焦點時停止播放並釋放裝置；返回前景或恢復焦點後重新協商。收到耳機拔除通知會關閉聲音，須手動重新開啟。無共同格式或所有候選格式失敗時釋放焦點並明確回報。

實作依據：[Android MediaCodec 的音訊初始化資料](https://developer.android.com/reference/android/media/MediaCodec)、[AudioTrack 非阻塞寫入](https://developer.android.com/reference/android/media/AudioTrack)、[音訊焦點與前景限制](https://developer.android.com/media/optimize/audio-focus)。音訊沒有增加外部原生庫或 FFmpeg 依賴。

## 環境

| 元件 | 設定／要求 |
| --- | --- |
| JDK | 17 以上；Java source／target 17，本輪使用 JDK 26.0.2 |
| Gradle／AGP | Wrapper 固定 9.7.1 並驗證官方 SHA-256；AGP 9.4.0 |
| SDK | compileSdk 36、targetSdk 36、minSdk 26；Build-Tools 35 以上 |
| ABI | arm64-v8a |
| Go | core module 要求 1.27，build 1524 的 AAR 記錄為 Go 1.27.0；gomobile／gobind 沿用 go.mod 鎖定版本 |
| NDK | 本輪使用 27.2.12479018；設有 16 KB linker 與成品檢查 |
| Python | 3.9 以上，僅標準函式庫 |
| 前端 Smoke | Node.js 22 以上及 Chromium／Edge／Chrome |

先設定 `JAVA_HOME`、`ANDROID_HOME` 與 `ANDROID_NDK_HOME`，並讓 Go、Python 可由 PATH 執行。可用 `YOURDESK_GO` 指定 Go。所有命令從 repository 根目錄執行；Windows 使用 `android/gradlew.bat`。

本版的 compileSdk／targetSdk 為 36。程式已使用系統 Insets 與 AndroidX OnBackPressedDispatcher 統一返回操作；Android 16 的實際行為仍須在對應裝置驗收。此流程交付內含 APK 的 ZIP，沒有執行 Play Console 上架。

## 一次完成 Release

```sh
python3 scripts/android_release.py build
python3 scripts/android_release.py verify --apk dist/android/YourDesk-1.26.1002-build-2301-android-arm64.apk
python3 scripts/android_release.py verify-zip --zip dist/android/YourDesk-1.26.1002-build-2301-android-arm64.zip
```

腳本依序驗證或重建 Go core／CameraX JNI，執行 Wrapper 的 `lintRelease`、`assembleRelease`，以 16 KB ZIP 對齊後簽章，再核對版本、非偵錯狀態、ABI、所有原生 ELF、ZIP 與 v2／v3 簽章。成品驗證成功才替換目標檔，再使用固定檔案清單與 Deflate 壓縮封裝 ZIP，內含原版簽章 APK、四語安裝說明、四語授權文件、BUILD.json 與 SHA256SUMS。每個檔案逐一核對 SHA-256；固定時間與權限使相同輸入產生相同 ZIP。封裝不重簽 APK。

```text
dist/android/YourDesk-1.26.1002-build-2301-android-arm64.apk
dist/android/YourDesk-1.26.1002-build-2301-android-arm64.sha256
dist/android/YourDesk-1.26.1002-build-2301-android-arm64.json
dist/android/YourDesk-1.26.1002-build-2301-android-arm64.zip
dist/android/YourDesk-1.26.1002-build-2301-android-arm64.zip.sha256
dist/android/YourDesk-1.26.1002-build-2301-android-arm64.zip.json
```

GitHub Release 僅上傳 `.zip`、`.zip.sha256`、`.zip.json` 三個檔案。APK 與其原始建置紀錄保留在本機；ZIP 的 JSON 另記錄 `archive.sha256`、大小及逐檔雜湊，頂層 `sha256` 仍為 APK。

已有通過驗證的正式 APK 時，可直接封裝並檢查：

```sh
python3 scripts/android_release.py pack --apk dist/android/YourDesk-1.26.1002-build-2301-android-arm64.apk
python3 scripts/android_release.py verify-zip --zip dist/android/YourDesk-1.26.1002-build-2301-android-arm64.zip
```

`pack` 會重新驗證 APK 簽章並比對旁邊的 `.json` 建置紀錄；若發行時另有補上來源提交資訊的紀錄，可用 `--record <檔案>` 指定，仍須與 APK 的版本、ABI、雜湊及憑證一致。

預設沿用本機 `android/cert/yourdesk-release.jks`、`android/cert/keystore-password.txt` 及 alias `yourdesk-release`。新 checkout 須由金鑰保管者提供既有金鑰，不應重新建立。也可指定：

```sh
python3 scripts/android_release.py build \
  --keystore /secure/path/release.jks \
  --password-file /secure/path/store-password.txt \
  --alias yourdesk-release
```

Key 密碼不同時加 `--key-password-file`。亦支援 `YOURDESK_ANDROID_KEYSTORE`、`YOURDESK_ANDROID_PASSWORD_FILE`、`YOURDESK_ANDROID_KEY_PASSWORD_FILE`、`YOURDESK_ANDROID_KEY_ALIAS`。密碼由 apksigner 讀取檔案，不寫入命令列；金鑰及密碼目錄已加入 Git ignore，仍應放在限制存取的儲存位置。

## 原生依賴與個別建置

```sh
python3 scripts/android_core.py build
python3 scripts/android_native.py build
android/gradlew -p android :app:lintRelease :app:assembleDebug :app:assembleRelease
```

`preBuild` 只驗證 core 與 CameraX 來源指紋及二進位，不會偷偷使用過期 AAR。`assembleRelease` 後還會驗證最終 APK 全部原生庫；`releasePreviewApk` 保留為相容入口，但輸出的 `preview-unsigned.apk` 未簽章。可安裝版本應使用上述 Release 腳本。

Go core 採獨立匿名暫存來源建置，消除 local replace 路徑被寫入 gomobile metadata 的問題，保留 `-trimpath`、prefix-map 與個人路徑掃描。AAR 與 manifest 成對發布，失敗回復舊產物並保留 `.bak`。需要強制重建時使用 `android_core.py build --force`。

CameraX 固定 1.6.2。上游 arm64 `libsurface_util_jni.so` 未通過本專案的嚴格 GNU_RELRO 對齊檢查，因此 `android_native.py` 以 `android/third_party/camera-core/provenance.json` 記錄的官方來源及 SHA-256 重新連結該庫，保留原 POM 的傳遞依賴，輸出到忽略追蹤的本機 Maven 目錄。Apache 2.0 授權與修改說明隨 APK 提供。其餘原生庫逐一驗證，不降低檢查條件。

Android 播放實際使用 MediaCodec／JPEG，因此移除只有版本探測用途的 FFmpeg 依賴。此變更不會清除或重建桌面 FFmpeg，也不應刪除既有依賴快取。

## Smoke 測試

```sh
(cd android/core && go test -mod=readonly -race -vet=all ./...)
(cd android/core/third_party/anet && go test -mod=readonly -race -vet=all ./...)
node android/tests/frontend-regression.mjs
bash android/tests/run-policy-tests.sh
VIDEO_TEST_JDK="$JAVA_HOME" bash android/app/src/testHost/run-video-tests.sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s scripts -p test_android_core.py -v
android/gradlew -p android :app:connectedDebugAndroidTest
```

手機需解鎖，且允許測試使用相機。裝置測試使用獨立 Debug 套件與自己的站台資料。一般裝置測試有 31 項，包含縮放／拖移、工具列操作、JPEG／H.264 畫面與座標，以及 Opus／AAC／PCM 實際解碼與 AudioTrack 播放進度；九項網路整合測試在未指定測試 Host 時會明確略過。音訊 Smoke 會播放短暫、低振幅的合成測試音，不變更手機媒體音量。

LAN 整合測試使用 `android/tests/smoke-host`，復用桌面端正式 TLS／P2P／音訊協定，提供合成影像／聲音、控制回應與 Shell 回音；不擷取桌面或系統錄音、不注入系統輸入，也不執行 OS Shell。先建立僅供測試的密碼檔，再於電腦啟動：

```sh
go run ./android/tests/smoke-host -listen <本機區網IP>:47829 -secret-file <臨時測試密碼檔>
android/gradlew -p android :app:connectedDebugAndroidTest \
  -Pandroid.testInstrumentationRunnerArguments.smokeHost=<本機區網IP>:47829 \
  -Pandroid.testInstrumentationRunnerArguments.smokeSecret=<相同臨時測試密碼>
```

只使用拋棄式測試密碼；結束後關閉測試 Host 並移除密碼檔。此測試驗證實際手機接收 JPEG／音訊、長按右鍵／滾輪控制、合成多螢幕切換、傳送 Unicode 控制、Shell UTF-8 回音、音訊協商／備援／靜音、背景暫停、重連及斷線回首頁。焦點與耳機拔除事件另以相同系統回呼驗證清理；不替代實際通話、耳機／藍牙切換、桌面 OS 擷取權限及輸入注入驗收。

## 本輪驗證與限制

2026-10-01 已在 ASUS_I01WD／Android 11 以簽章輪替將原 `0.1.0` 升級為正式 build 1524，未卸載或清除資料。安裝前後 UID 維持一致；原有一個站台及記憶密碼保留，使用原設定成功開始真實遠端串流。手機上的最終 APK 與 `dist/android/YourDesk-1.26.1002-build-2301-android-arm64.apk` 的 SHA-256 完全一致，且非 Debug。ADB 擷圖與 UI 邊界確認關閉按鈕位於標題列右上角，沒有遮住螢幕選單；安裝紀錄及截圖位於 `.local-run/android-install/`。

本版（build 1524）修正串流開始後關閉按鈕偏移：隱藏期間不再以舊邊界累積位移，預設位置隨標題列及系統安全區配置；拖曳位置以安全區比例保存。先在同一真機重現「返回首頁、轉直向、重新串流」的錯位，再完成 17 項畫面及操作 Smoke（全部通過、無略過），其中三項新增驗證涵蓋重連、標題列高度／安全區變更、全螢幕、拖曳旋轉及實際點擊關閉；另以截圖像素確認旋轉動畫結束後的顯示位置。完整結果保留於 `.local-run/android-close-smoke-results.xml`，視覺結果於 `.local-run/android-close-visual-smoke-results.xml`，截圖於 `.local-run/android-close-fixed.png`。Release lint 為零 error／fatal、30 項 warning，正式 APK 的版本、v2／v3 簽章及 16 KB ELF／ZIP 檢查通過。

前一版（build 1449）於 2026-10-01 使用 ASUS_I01WD、Android 11／API 30、4 KB pagesize 真機；裝置的 28 項功能測試與 9 項 Wi-Fi 協定測試共 37 項全部通過（無略過）。音訊驗證包括 Opus／AAC／PCM 的非靜音解碼與實際 AudioTrack 播放進度、Wi-Fi 自動協商、來源編碼失敗備援、喇叭開關、無共同格式的錯誤與重試、焦點暫停、進入背景、耳機拔除回呼、斷線清理與重連。縮放測試涵蓋實際 JPEG 像素、H.264 靜止畫面立即重繪、觸控座標、多指尾端事件隔離，以及工具列放大／縮小／拖移／恢復完整畫面。新增驗證涵蓋長按右鍵、取消後無誤觸、雙向遠端捲動、滑鼠左／右／中鍵配對與小量滾輪累積、三個合成螢幕切換、確認與影像分開抵達時的輸入隔離、螢幕移除，以及 H.264 換螢幕等待關鍵影格與實際解碼。縮放選單收斂為五個必要項目，模式互斥且可取消勾選恢復點擊。主機端可視區幾何、音訊封包／世代／缺包排序及 Android core race Smoke（含螢幕回報世代、舊確認、逾時與恢復）通過；前端 25 案通過；前輪解碼 10,608 斷言及 Python 建置 16 案亦通過。完整 Gradle、Release lint 及正式 APK 的 v2／v3 簽章／16 KB ELF＋ZIP 檢查通過；lint 沒有 error，仍有非阻擋 warning。

最終手機結果記錄於 `android/app/build/reports/androidTests/connected/debug/index.html`；Release lint 位於 `android/app/build/reports/lint-results-release.html`。兩者為本機建置產物，不隨 Git 追蹤。

公開的 Android Release 使用 `android-<版本>` 標籤，附上內含 APK 的 ZIP、ZIP SHA-256 與封裝紀錄，並設定 `latest=false`。桌面更新程式使用 `/releases/latest`，因此該入口維持桌面版；Android 下載入口由 README 明確連至對應發行頁。本機站台、憑證、ADB 備份與使用者桌面截圖不列入提交或發行附件。

發行內文遵循[共同 Release 流程](RELEASE-WORKFLOW.md)，儲存於 `docs/RELEASE-<tag>.md`，從 `## 繁體中文` 開始，依序提供繁體中文、英文、日文、韓文。四語均須包含功能、安裝與升級注意事項、驗證範圍及已知限制；發布後讀回核對。本版來源為 [RELEASE-android-1.26.1001-build-1524.md](RELEASE-android-1.26.1001-build-1524.md)。修正既有發行的翻譯時，只更新內文，保留原 APK、標籤及發布狀態。

16 KB 靜態檢查不能取代 [16 KB 裝置啟動與 JNI 驗證](https://developer.android.com/guide/practices/page-sizes)。Android 16 實機、HEVC／不同廠牌解碼器、平板／折疊螢幕、真實通話／耳機／藍牙切換、跨行動網路切換及長時間負載仍需對應環境驗收，不能由單一 Android 11 手機推論全部完成。音訊已驗證非靜音 PCM 與系統播放進度，沒有使用麥克風回錄喇叭輸出。

本次 Android 與桌面版合併至同一正式 Release；桌面版 latest 中包含 Android ZIP 不影響桌面平台附件選取。獨立 Android 發行仍採 android- 前綴與 latest=false。
