# 組合按鍵修正與發行驗證

日期：2026-11-05。版本：[1.26.1105 build 1048](RELEASE-1.26.1105-build-1048.md)。Android versionCode：`29897448`。

## 範圍

本輪合併桌面既有組合按鍵修改與 Android 補齊：macOS Viewer 擷取／本機快捷鍵狀態、macOS Host 修飾旗標、Windows E0 與功能鍵注入、Windows／macOS 映射、虛擬鍵盤明確選鍵，以及 Android 原始鍵能力公告、實體掃描碼／修飾鍵快照／失焦重設。Go 工具鏈固定為既有工作目錄使用的 `go1.27.3`，不更新第三方功能依賴。WinPE 封裝相容 Go 工具鏈將授權檔放在 GOROOT 或安裝根目錄的配置，確認內容屬於 Go 授權後才納入。操作與限制見[系統快捷鍵](SYSTEM-SHORTCUTS.md)與 [Android 建置](ANDROID-BUILD.md)。

## Smoke 與發行閘門

Android 修正已完成 31 項隔離 Chromium Smoke，包含漏收到獨立 Ctrl 按下時的 Ctrl+C、Ctrl+Shift+方向鍵、Alt+Tab、功能鍵、標點、Meta、缺少 code 的方向鍵、失焦重設、中文 IME 與舊 Host 相容性。橋接為替身，未連接實際遠端 Host。Android core 測試、AAR 來源／個人路徑／16 KB 檢查及 Debug／Release APK 編譯通過，APK 內前端與原始碼一致；發行準備已比對原始碼雜湊。

桌面發行前檢查 `internal/rawkey`、`internal/input`、`cmd/remote` 的既有測試與原生編譯，正式發行重新建置 macOS arm64、Windows x64／ARM64、Linux x64／arm64 CLI、WinPE x64 與 Android arm64。確認所有檢查成功後才公開附件。

套件閘門涵蓋：版本／架構／SHA-256；Windows Portable 與服務 ZIP、DLL 與服務 manifest；正式 DMG 內 App 的啟動 Smoke、Developer ID 簽章、Apple 公證、票證及 Gatekeeper；Android Release lint、既有正式憑證、遞增版本碼、APK v2／v3 與 16 KB 對齊。GitHub 草稿的 16 個附件大小與 digest 必須全部一致，正式發布後再核對 latest、四語內文與標籤提交。

本機證據位於 `.local-run/android-chords-20261105/`、`.local-run/keyboard-release-smoke-20261105/` 與 `.local-run/release-1.26.1105-build-1048/`，不提交本機日誌、備份或憑證。

## 驗證界線

編譯、瀏覽器替身及套件驗證不代表 Android 外接鍵盤到 Windows／macOS 的完整實機驗收；不同鍵盤配置、IME、Android 系統攔截及 macOS 系統保留快捷鍵仍需對應環境驗證。兩端更新後才能套用完整的發送／接收修正。Windows 安全桌面與 Ctrl+Alt+Del 仍需既有已授權服務；本版未新增繞過 UAC 或 SAS 原則的方式。

本輪不重述 10 月 20 日手機串流結果為本版實機結果；長時間網路、Windows／WOA 驅動／服務更新、Linux／WinPE 實機、Android 16／16 KB 裝置仍保留原驗證限制。遠端聲音、登入前服務與 WinPE 維持實驗性。
