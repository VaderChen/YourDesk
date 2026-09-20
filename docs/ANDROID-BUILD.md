# Android 建置與驗證

本文件對應 `android/` 的獨立 Android Client，更新於 2026-09-21。桌面版的 Go module、FFmpeg 建置及測試，不能替代 Android core／APK 的驗證。

## 目前可交付狀態

- [深度檢查](ANDROID-DEEP-REVIEW-2026-09-20.md) 的 17 項問題中，16 項已完成原始碼修正與適用的主機端驗證；詳見[修正紀錄](ANDROID-REVIEW-FIXES-2026-09-20.md)。
- **A02 尚未完成：先前本機保留的 `android/libs/androidcore.aar` 是舊核心，ELF 為 4 KB 對齊；沒有產出包含此次修正的新 AAR／APK。** 不可把來源碼同步或主機測試通過當成 Android 發布完成。
- AAR 與產生的 `androidcore-sources.jar` 由本機建置產生，不隨 Git 分發。新 checkout 必須依下列流程建置；既有本機產物保留，僅在新產物驗證成功後替換。
- 上次完整建置受限於缺少可用 Android NDK、Gradle 9.6.0 下載未完成；APK 建置、lint、instrumentation 與真機驗收仍待執行。
- 新增的建置檢查會拒絕缺少來源指紋、內容不符或不符合 16 KB ELF 要求的 AAR。請完成下列正常重建流程，不要停用 gate 或手動偽造 manifest。

## 環境與版本

| 元件 | 專案設定／要求 |
| --- | --- |
| JDK | 17 以上；Java source／target 為 17 |
| Android Gradle Plugin | `android/build.gradle.kts` 固定 9.4.0；對應建置環境採 Gradle 9.6.0，完整任務仍待驗證 |
| Android SDK | compileSdk／targetSdk 35；minSdk 26 |
| ABI | 僅 `arm64-v8a` |
| Go | `android/core/go.mod` 要求 1.27；依 module 固定的依賴建置 |
| gomobile／gobind | 由 `android/core/go.mod` 的 `golang.org/x/mobile` 版本決定；不使用全域任意版本 |
| NDK | 需可用的 Android NDK，供 gomobile／cgo 產生 JNI；最終仍須通過原生庫檢查 |
| Python | Python 3.9 以上，使用標準函式庫執行 `scripts/android_core.py` |
| 瀏覽器測試 | Node.js 22 以上及已安裝的 Chromium／Edge／Chrome；不需要 npm 安裝 |

目前沒有 Gradle wrapper，以下使用已安裝且符合上述版本的 `gradle`。SDK、NDK、JDK 安裝與授權須由開發者按自己的環境完成；腳本不代為接受 SDK 授權。

先設定 `JAVA_HOME`、`ANDROID_HOME`；需要指定 NDK 時設定 `ANDROID_NDK_HOME`。將 Go、Java、Gradle 放入 `PATH`，或以 `YOURDESK_GO` 指定 Go 執行檔。以下專案命令皆從 repository 根目錄執行，不依賴固定使用者或磁碟路徑。

## 1. 重建並驗證 Android core

```sh
python3 scripts/android_core.py build
python3 scripts/android_core.py verify
```

`build` 的行為：

1. 先驗證既有 AAR；來源指紋、二進位指紋及原生庫檢查都通過時，直接保留，不重新建置。
2. 有變更或驗證不符時，使用 module 鎖定的 gomobile／gobind 建置 arm64、API 26 的候選 AAR，加入 16 KB linker 設定。所有 Go 建置套用 `-trimpath`；C／C++ 使用 prefix-map 將專案、工具鏈及暫存目錄映射為相對路徑。
3. 核對來源在建置期間沒有變動，檢查 ELF 的架構、PT_LOAD 與 GNU_RELRO，並拒絕含個人目錄的 native library、Java class 或 AAR metadata，再建立 `android/libs/androidcore.manifest.json`。作業系統必要路徑不屬於個人建置路徑。
4. 正式替換前建立 `.bak`，替換後再次驗證；失敗則回復原產物並保留恢復用備份，成功才移除備份。

來源指紋包含 core 的 production source、native／embed 資源及建置腳本；測試、隱藏檔與暫存備份不計入。AAR 與 manifest 必須成對保留；不要在來源修改後沿用舊 manifest。

僅在需要強制重建時使用 `python3 scripts/android_core.py build --force`。發現既有 `.bak` 或發布鎖時，先確認沒有另一個建置程序並檢查上次失敗原因，不要盲目刪除保護檔案。

## 2. 完整 Gradle 建置

先成功完成 core 驗證，再執行：

```sh
gradle -p android :app:lintDebug :app:assembleDebug
gradle -p android :app:releasePreviewApk
```

- `preBuild` 會呼叫 `verifyAndroidCore`，只驗證、不自動重建。若需要從 Gradle 觸發 core 重建，可先執行 `gradle -p android :app:buildAndroidCore`。
- 若環境的 Python 命令不是 `python3`，可附加 Gradle property，例如 `-Ppython=python`。
- `assembleRelease` 完成後會執行 `verifyReleaseNativeLibraries`，檢查最終 APK 內全部 arm64／x86_64 原生庫的 ELF；未壓縮 `.so` 還會檢查 ZIP data 的 16 KB 對齊。
- `releasePreviewApk` 只有在最終原生庫檢查通過後才複製預覽檔，不繞過檢查。

目前版本設定仍為 `0.26.0915 build 1801`，輸出位置為：

```text
android/app/build/outputs/apk/debug/app-debug.apk
android/app/build/outputs/apk/release/app-release-unsigned.apk
android/app/build/outputs/apk/release/YourDesk-0.26.0915-build-1801-preview.apk
```

**Release 與上述 preview APK 都未簽章。** 檔名中的 preview 不代表已簽章、可直接安裝或通過裝置驗收；正式簽章／版本發布另行處理。簽章或其他重新封裝後，對實際要交付的 APK 再執行一次：

```sh
python3 scripts/android_core.py verify-apk --apk android/app/build/outputs/apk/release/app-release-unsigned.apk
```

上例為建置產物；驗證正式交付檔時，將 `--apk` 改成該檔案的相對路徑。原生庫靜態檢查通過仍不能取代 [Android 16 KB 裝置測試](https://developer.android.com/guide/practices/page-sizes)。

## 3. 可重跑的主機端回歸

```sh
(cd android/core && go test -mod=readonly -race -vet=all -count=3 ./...)
(cd android/core/third_party/anet && go test -mod=readonly -race -vet=all -count=10 ./...)
node android/tests/frontend-regression.mjs
bash android/tests/run-policy-tests.sh
VIDEO_TEST_JDK="$JAVA_HOME" bash android/app/src/testHost/run-video-tests.sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s scripts -p test_android_core.py -v
```

`anet` 是巢狀獨立 module，不能省略第二條 Go 命令。Go race 測試在支援 race detector 且已備妥 C toolchain 的主機執行；跨編譯成功不代表目標平台執行成功。

瀏覽器 runner 自動尋找常見 Chromium 瀏覽器，也可用 `BROWSER` 指定執行檔。它只使用獨立暫存 profile、本機 assets 與 mock Native bridge，不使用個人瀏覽器資料或連線到真實 Host。Java 主機測試可用 `VIDEO_TEST_JDK` 指向既有 JDK，不必變更系統 Java。

2026-09-20 修正階段的驗證紀錄：

| 驗證 | 已記錄結果 | 不代表 |
| --- | --- | --- |
| Android core＋anet | 23 個 Go 測試，含 race、高壓慢消費、缺 keyframe 與取消競態 | 真實手機網路／Host 整合完成 |
| 前端 | 16 個 headless Edge 案例 | Android WebView／各廠 IME 真機行為相同 |
| 解碼主機測試 | 10,608 項斷言，128 MiB JVM 下累計 512 MiB AU 處理 | 真實 MediaCodec 硬體效能或相容性已驗證 |
| Python 建置檢查 | 10 個測試，含替換失敗回復與原生庫檢查 | 新 AAR 已建成或 APK 可執行 |
| 真實 Android API 型別編譯 | 全部 main／androidTest 已用 API 35 與實際依賴編譯通過 | 完整 Gradle、lint、instrumentation 已執行 |

Java 解碼主機測試使用 API fake；另一項真實依賴編譯不使用 fake。兩者目的不同，不混稱為實機測試。

## 4. 裝置驗收

在已備妥 SDK／adb、AAR 與 APK 的環境，使用專用測試裝置執行：

```sh
gradle -p android :app:connectedDebugAndroidTest
```

這會建置、安裝並執行裝置測試，請勿對使用中的遠端工作裝置直接操作。另需人工核對：

- 不同廠牌／codec 的 JPEG、H.264、HEVC 能力協商、解碼失敗降級與高壓影格恢復；Android 尚不支援 AV1，不應宣告或選用 AV1。
- 靜止桌面 Surface 重建、背景返回、JPEG 丟塊後完整基底恢復。
- Shell 控制鍵、中文組字／取消／大段貼上及新舊 Host 相容；中文 committed text 功能需要支援新協定的 Host。
- 自架 signaling、Wi-Fi／行動網路切換、取消後立即重連。
- 相機權限允許／拒絕／取消、Activity 銷毀、背景返回及 QR 掃描停止。
- 畫質／75%、60%、50% 比例設定與 Host ACK 一致。
- 16 KB page-size 環境的啟動、JNI、FFmpeg 依賴及解碼路徑。

## 快取與既有二進位保留

Android core 的建置不會重建或清除桌面 FFmpeg。Android FFmpeg 目前使用 Gradle 預編譯依賴 `com.mrljdx:ffmpeg-kit-full:6.1.4`，應保留已取得的產物；沒有來源／設定變更或驗證失敗時，不為一般重跑而清除其快取。FFmpeg runtime 探測不等於已實作軟體影格解碼 fallback。

主機測試會清理自己建立的暫存輸出；一般驗證不需先執行 `clean`，也不要刪除 `android/libs/`、來源 manifest 或既有 FFmpeg 二進位。需要清除中繼檔時應先限定目標，不要把保留的依賴／二進位一起移除。
