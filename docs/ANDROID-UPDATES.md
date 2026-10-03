# Android APK 自動更新

本功能首次隨 [1.26.1003 build 2028](RELEASE-1.26.1003-build-2028.md) 發行。既有 Android `1.26.1002 build 2301` 尚未包含更新器，必須先手動安裝一次本版正式簽章 APK，之後才能由 App 偵測並下載後續更新。首頁版面、站台資料與遠端操作維持原狀。

## 使用方式

正式版開啟後，在首頁前景且沒有對話框、QR 掃描或連線作業時，自動查詢官方 GitHub Releases。正常檢查間隔為六小時；網路或驗證失敗後十五分鐘重試。未在首頁前景時停止工作，回到首頁再依排程執行；沒有常駐服務、背景通知或輪詢執行緒。

發現新版便自動下載 ZIP，驗證並取出 APK。完成後沿用目前的繁體中文、英文、日文或韓文顯示原生「新版已下載」提示。選「稍後」會保留 APK，六小時後在首頁再次提示；關閉 App 後也能恢復已驗證的完整下載。中斷的下載會清除暫存，下次重新下載。

選「安裝更新」後，若尚未允許 YourDesk 安裝套件，會開啟 Android 的「允許來自此來源」設定；授權後返回 App，再交給系統安裝程式確認。此實作自動偵測與下載，安裝仍由使用者確認。同一 applicationId 與相容簽章的更新保留站台及設定，不需解除安裝舊版。拒絕權限或取消安裝不會清除已下載的 APK。

Debug 套件 `com.yourdesk.android.debug` 不會從正式發行來源自動下載或安裝，避免混用測試資料與正式簽章。

## Release 格式

更新器讀取官方 [GitHub Releases API](https://docs.github.com/en/rest/releases/releases#list-releases)，最多十頁、每頁二十筆，不依賴可能只有桌面資產的 `/latest`。略過 draft、prerelease、舊版與附件不完整的發行，支援以下兩種 tag：

```text
1.26.1003-build-1500
android-1.26.1003-build-1500
```

以上只示範命名，不表示該版本已發布。發行附件繼續使用既有 `scripts/android_release.py` 的三個檔案：

```text
YourDesk-<版本>-build-<時間>-android-arm64.zip
YourDesk-<版本>-build-<時間>-android-arm64.zip.sha256
YourDesk-<版本>-build-<時間>-android-arm64.zip.json
```

ZIP 與 JSON 必須完整上傳，並由 GitHub 提供 `sha256:` 資產 digest。JSON 保留既有欄位：`applicationId`、`abi`、`versionName`、`versionCode`、`apkFile`、APK `sha256`、`signerSha256`，以及 `archive.fileName`、`archive.sha256`、`archive.size`、`archive.entries`。APK 必須是 arm64 正式版；每次發行的顯示版本及 versionCode 都須增加。桌面單獨發行與 Windows ZIP 更新流程不受影響。

## 驗證與資源限制

- 只接受此專案的 HTTPS 發行連結及 GitHub 官方資產重新導向，不傳送站台、密碼或其他認證資料。
- 先核對 GitHub 的 JSON 大小與 digest，再比對 ZIP 大小／digest、下載內容與 APK SHA-256；最後由 Android 解析套件 ID、版本、非 Debug 狀態、最低系統版本及簽章。
- APK 的簽署者必須符合發行紀錄，並與目前 App 的簽章相同，或具有 Android 可辨識、包含目前簽章的向前金鑰輪替紀錄。系統安裝程式仍會執行自身檢查。
- JSON feed 每頁上限 2 MiB、manifest 64 KiB、ZIP 128 MiB、APK 256 MiB。下載及解壓使用 64 KiB 緩衝區，不把完整 ZIP／APK 載入記憶體。ZIP 最多 32 個根目錄檔案，其他說明或授權檔每個最多 2 MiB。
- 拒絕路徑跳脫、重複項目、額外 APK、過大內容、截斷或雜湊不符。只將指定 APK 寫入 App 私有更新目錄；不依 ZIP 內路徑建立檔案。
- 連線或離開首頁時取消下載，失效的工作結果不會彈出提示。完整 APK 與紀錄以原子替換寫入；重啟時重新核對雜湊、版本及簽章，刪除不完整、損壞或已過期的快取。
- APK 經由非公開的 [FileProvider](https://developer.android.com/reference/androidx/core/content/FileProvider) URI 授權系統安裝程式讀取；不要求共用儲存空間權限。安裝來源權限依 [PackageManager.canRequestPackageInstalls](https://developer.android.com/reference/android/content/pm/PackageManager#canRequestPackageInstalls()) 判斷。

## 維護與驗收

核心測試使用 JUnit；瀏覽器測試確認對話框、語言、頁面切換及背景狀態會正確通知更新器，並保留原本的站台／遠端輸入回歸案例：

```sh
python3 scripts/android_core.py build
python3 scripts/android_native.py build
android/gradlew -p android :app:testDebugUnitTest :app:lintRelease :app:assembleDebug :app:assembleRelease
node android/tests/frontend-regression.mjs
```

本版尚待完成的實機驗收：以兩個遞增版本及既有正式簽章確認首頁下載、連線與背景取消、冷啟動恢復、拒絕／允許安裝來源、取消／完成安裝，以及更新後站台和密碼仍可使用。此限制已列於四語發行說明。主機 JVM 與瀏覽器測試不代表已完成 Android 系統安裝驗收；未簽章 APK 也不能直接當作正式更新發布。

### 2026-10-03 驗證結果

- Gradle 9.7.1、JDK 17.0.20.1、SDK 36：`testDebugUnitTest` 的 25 項測試、`lintRelease`（零錯誤）、Debug／未簽章 Release 建置全部通過；原生 ELF／ZIP 對齊及來源指紋檢查通過。
- Chromium 首頁／遠端操作回歸 26 項通過，包含新增的更新閒置狀態、對話框、四語及前背景切換；既有 frame／viewport／audio policy 與 video host 測試亦通過。
- 實際讀取 GitHub 發行資料，下載 15,708,017 bytes ZIP，解出 33,594,743 bytes APK；在主機 JVM `-Xmx16m` 下完成大小與 SHA-256 驗證。Android SDK `apksigner` 驗證該已發布 APK 的 v2／v3 簽章，套件 ID、版本及憑證均與 manifest 相符。此記憶體上限只代表串流下載／解壓測試，不代表完整 Android App 的記憶體用量。
- macOS 外接磁碟產生的 `._` 中繼資料會干擾 Android 資源解析；本輪將 Gradle 建置輸出及快取放在本機暫存目錄完成驗證。APK 內容另確認沒有 `.bak` 或 `._` 檔案。
- 後續發行使用提供的原正式金鑰，完成 `1.26.1003 build 2028` APK 的 v2／v3 簽章、原憑證延續性、版本與 ZIP 雜湊檢查，與桌面版合併發布。未連接 Android 裝置，尚未完成本輪實機覆蓋安裝與系統更新驗收。
