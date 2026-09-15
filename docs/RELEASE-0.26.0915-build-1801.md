# YourDesk 0.26.0915 build 1801

## 發行說明

這是 **測試／預覽版本**，提供給內部驗證使用。仍有許多問題與功能尚未處理完成，不代表正式穩定版。

### 本次內容

- Android Viewer 更新至 `0.26.0915 build 1801`。
- 修正 Android Shell/XTERM 使用鍵盤時的可用高度：Shell 會避讓鍵盤，GUI 維持原本的覆蓋行為。
- 修正列表站台名稱與 ICON 的間距。
- 整合近期桌面端、剪貼簿、影像與輸入處理變更。

### 已驗證

- Android Release 編譯與 `lintVitalRelease` 通過。
- Android Shell 實機 Smoke：鍵盤關閉 47 列，開啟 27 列；XTERM 底部與鍵盤頂端一致。
- Release APK 已完成 APK Signature Scheme v2/v3 簽署與 zipalign 驗證。

### 已知限制

- 目前仍屬預覽版，跨平台連線、影像解碼、剪貼簿、檔案傳輸、遠端 Shell、相機掃描及背景生命週期等功能尚未完整驗證。
- Android 目前只封裝 `arm64-v8a`。
- 不應將本版本視為所有裝置或所有網路環境均可用的正式版本。

## English

This is a **test/preview build** for internal validation. Many issues and features remain unresolved; it is not a stable production release.

The Android Viewer includes the keyboard-aware Shell/XTERM layout fix and the site-list icon spacing fix. Android Release build, lint, real-device keyboard smoke checks, APK v2/v3 signing and zip alignment were verified. Android currently ships only `arm64-v8a`. Cross-platform connectivity, media, clipboard, file transfer, remote Shell, camera scanning and lifecycle behavior still require broader validation.
