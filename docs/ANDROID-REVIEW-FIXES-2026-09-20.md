# Android 檢查修正紀錄

對應 [Android 深度檢查](ANDROID-DEEP-REVIEW-2026-09-20.md) 的 17 項問題。修正驗證紀錄日期為 2026-09-20；文件於 2026-09-21 整理。正式建置前請參閱 [Android 建置與驗證](ANDROID-BUILD.md)。

## 狀態

16 項已完成原始碼修正並做適用的主機端驗證；**A02 的舊 Go AAR 尚未重建，不能視為 16 KB／APK 驗收完成**。下列結果是修正階段的驗證紀錄，該階段沒有安裝 APK、操作手機或連線到使用者的 Host。後續同步來源碼與文件，不代表已產出新版 AAR／APK 或通過裝置驗收。

| 項目 | 修正 | 驗證／限制 |
| --- | --- | --- |
| A01 佇列超限 | 同時限制 512 張及 32 MiB；超限清除相依鏈，等待完整影格，回呼僅設定恢復旗標 | 慢讀取、100,000 張缺關鍵幀、race 測試通過 |
| A02 16 KB AAR | 加入 pinned gomobile 建置、來源／二進位指紋、ELF／RELRO／ZIP 檢查及失敗回復；Gradle 阻擋舊 AAR | 10 個建置／故障注入測試通過；**仍缺 NDK，未產出新 AAR** |
| A03 CSD | AVC 補 Annex B 起始碼；HEVC 的 VPS／SPS／PPS 合併 `csd-0` | 真實 H.264 fixture＋API 契約測試；手機硬解測試已新增、未執行 |
| A04 能力／降級 | 公告與硬解路徑一致，驗證尺寸／profile，有界嘗試候選；失敗撤回 codec，協商失敗會節流重試 | Java 契約測試＋整合編譯；真實 Host／裝置降級待測 |
| A05 Shell | 統一由 xterm textarea 處理輸入，不再有平行 password proxy 吞鍵 | Edge 真實 xterm 控制鍵、組字、大段貼上通過 |
| A06 分片放大 | 限制 payload／總 bytes、拒絕空片段、按實際大小配置、已完成序號去重 | Go 回歸與 race 通過 |
| A07 input buffer | 容量不足、null、queue／drain 例外均釋放 decoder；輸入飢餓有界切換候選 | API fake 所有權／失敗回復測試通過；不是硬體測試 |
| A08 JPEG 基底 | 明確有效／失效狀態，缺一塊後持續拒絕差分，直到完整 keyframe | 序列 10→12→13→完整影格回歸通過；真實 Bitmap instrumentation 已編譯 |
| A09 JPEG 配置 | `inJustDecodeBounds` 預查 MIME、尺寸、像素、貼圖範圍；完整影格直接作可變基底，避免額外完整複製 | 邊界／溢位策略測試；保留原 32M 像素範圍，不默默禁用 6K |
| A10 換網 | 不再永久快取 IP／錯誤，每次重新取得路由，增加 IPv6 路徑 | 路由失敗後重試／IPv6 測試；實機換網待測 |
| A11 取消 | 原子註冊 cancel＋連線世代，舊回呼不能污染新佇列／能力 | 4,000 次並行 Connect／Close、race 通過 |
| A12 IME | committed text 專用有界協定，macOS／Windows Unicode 注入，不動剪貼簿 | Edge／Go dispatch 驗證；舊 Host 保留 ASCII，中文明示需更新 |
| A13 signaling | 站台 signal 傳到底層，驗證 WSS／HTTPS，空值才用預設；換 room 不沿用先前站台 | Java URL 邊界與瀏覽器 payload 測試通過 |
| A14 相機執行緒 | 啟停、權限及 View 操作集中主執行緒 | 真實 CameraX 型別編譯；手機權限／取消待測 |
| A15 相機生命週期 | 改綁 Activity lifecycle，銷毀清理，世代拒絕過期 future／掃描回呼；授權後恢復待開始的掃描 | 靜態交叉審查＋真實依賴編譯；裝置 lifecycle 待測 |
| A16 畫質／比例 | 完整 `stream-config` 快照，明確 100／75／60／50；核對 Host session／revision ACK，區分等待、成功、拒絕與逾時 | 真正 `keyboard-capabilities` envelope 回歸；同 revision 的 effective 更新仍接收 |
| A17 Surface | 重建或等待 IDR 時持續節流要求完整影格，不再只對負回傳值恢復 | decoder reset／等待 IDR 契約測試＋整合審查；靜止桌面實機待測 |

## 額外改善

- `ReadFrameJSON` 先出列、釋放接收鎖，再做 JSON／Base64；仍是 JSON 管線，不宣稱零複製。32 MiB 是佇列預算，不是 APP 總記憶體上限。
- Android control sender 的狀態檢查、排隊與傳送有序；滿載回報錯誤，不淘汰已接受的按鍵事件。
- UI frame pump 限制單輪最多 4 筆及 8 ms 的迴圈預算，每 16 ms 再輪詢，統計包含 MediaCodec drain 的輸出。單張 JPEG 解碼仍可能超過迴圈預算，完整背景解碼管線仍是後續效能工作。
- `.gitignore` 原先 `/android/` 規則會隱藏新增來源檔，但既有 65 個 Android 檔案已追蹤；改為只排除工具快取、建置產物、本機設定，使新來源與回歸測試可納入版本管理。
- 保留所有既有桌面修改；Host 僅新增 Android IME 所需的 Unicode 注入與能力公告。
- 專案內 FFmpeg 二進位及 Go AAR 均未替換。額外讀取正式 FFmpeg Kit 6.1.4 arm64 依賴，10 個 `.so` 的 PT_LOAD 均為 16 KB 對齊，沒有因此重建 FFmpeg。

## 已執行

1. Android core＋anet：23 個正式 Go 測試；core `-race -vet=all -count=3`、anet `-race -count=10`。含 10,000 張各 64 KiB payload 的慢消費、100,000 張缺 keyframe、4,000 次 Connect／Close。
2. 16 個獨立 headless Edge 案例：控制鍵、中文組字及取消、事件順序、16 KiB UTF-8 分片、大段貼上、舊 Host 相容及 signal。測試不使用個人瀏覽器 profile。
3. Decoder host test：**10,608 assertions**；JVM `-Xmx128m` 下累計 512 MiB AU 處理。這一套使用 Android API fake，是契約／邏輯壓測，不是硬體解碼效能。
4. 真實型別編譯：官方 Android API 35 `android.jar`＋實際 AndroidX／CameraX／MLKit／FFmpegKit／JUnit＋最新 gobind Java，`javac --release 17` 編譯全部 main／androidTest 成功。這次編譯沒有用 fake 取代 API；有 deprecated API 警告。
5. FramePolicy：100,000 張缺幀策略、6K 相容與 JPEG 幾何／溢位；SignalingAddress：預設、自架 WSS／HTTPS、IPv6、scheme、憑證字串／控制字元拒絕。
6. Python 建置 gate：10 個測試，含 manifest 替換失敗、驗證失敗 rollback、既有備份／鎖保護、native 資源指紋、ELF／RELRO／ZIP。
7. 桌面 `internal/input`、`internal/idleclose`、`internal/hostsession`、`internal/p2p` 的 race／vet 通過。Windows input 與 Android Go core 的跨編譯通過，不等於目標平台執行。
8. `git diff --check`、自有 JS 語法檢查通過。

## 尚未完成與原因

- **Android NDK／JNI／AAR 重建**：沒有可用 NDK。既有 `android/libs/androidcore.aar` SHA-256 保持 `1e6595c3137b2a1fd18cb5557a22576c12660c07f5518f10f0481a1e1768b75a`，仍是檢查時的 4 KB 舊核心，不能配上新增 Go API 直接出新版 APK。
- **完整 Gradle／APK**：官方 AGP 9.4.0 POM／source 可解析，並按其原始碼修正 variant task 延後註冊的時序；官方 Gradle 9.6.0 下載 60 秒僅取得約 3 MB 後停止，未完成 `help`／`tasks`／`assemble`／lint。沒有擅自更換 AGP，也沒有執行 SDK 授權接受命令。
- **真機與 16 KB 執行**：未做 APK 安裝、不同廠牌 codec、背景返回、相機權限、換網、Unicode 原生注入或 16 KB 手機驗收。
- **發布條件**：即使完成 Go AAR ELF 檢查，也必須讓 APK 最終檢查檢驗全部原生依賴及 ZIP 對齊，再進行裝置測試。[Android 16 KB 指引](https://developer.android.com/guide/practices/page-sizes)。

## 重跑方式（從專案根目錄）

設定可用 JDK 的 `JAVA_HOME`，或以 `VIDEO_TEST_JDK` 指向既有 JDK；腳本沒有硬編碼使用者／磁碟路徑。

```sh
node android/tests/frontend-regression.mjs
bash android/tests/run-policy-tests.sh
VIDEO_TEST_JDK="$JAVA_HOME" bash android/app/src/testHost/run-video-tests.sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s scripts -p test_android_core.py -v
```

分別在 `android/core` 與 `android/core/third_party/anet` 執行：

```sh
go test -mod=readonly -race -vet=all ./...
```

備妥 Android SDK／NDK／JDK，設定 `ANDROID_HOME`、必要時 `ANDROID_NDK_HOME`，Go 放在 PATH 或設定 `YOURDESK_GO` 後：

```sh
python3 scripts/android_core.py build
python3 scripts/android_core.py verify
```

`build` 使用 `android/core/go.mod` 鎖定的 gomobile/gobind，只有來源或驗證不符才建置，成功驗證後才替換 AAR；失敗保留原產物及恢復用 `.bak`。不更動 FFmpeg。執行 `gradle -p android :app:releasePreviewApk` 前，須具備相容 Gradle／SDK；新的 preBuild gate 不允許以未驗證舊 AAR 繼續。建置流程已設定最終 unsigned release APK 的全部 native library／ZIP 檢查，但尚未有本次新版 APK 的實際通過紀錄；preview 亦未簽章。

完整裝置回歸入口為 `gradle -p android :app:connectedDebugAndroidTest`；應在測試裝置執行，不在使用中的遠端工作階段操作。
