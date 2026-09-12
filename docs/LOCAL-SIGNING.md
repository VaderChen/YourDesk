# 本機 clientUI.app 簽章

`clientUI.app`、`bin/yourdesk-client` 與 `bin/yourdesk-remote` 均使用 Developer ID Application 簽章，拒絕退回 ad-hoc。三者沿用既有簽章識別碼 `clientUI`、`yourdesk-client`、`yourdesk-remote`；啟動器另明確寫入相同的 CFBundleIdentifier，避免因產生方式不同而改變身分。

- `scripts/create-client-ui-launcher.sh`：在暫存目錄產生啟動器、簽署、送 Apple 公證並附加票根，全部成功後才替換專案下的 clientUI.app。沿用正式建置的 `YOURDESK_NOTARY_PROFILE`（預設 `VaderApp`），不在腳本保存帳密。
- `runUITest.command`：在暫存目錄建置並簽署兩個 Go 執行檔，再移到固定的 bin 路徑，最後啟動介面。原路徑不會在重建期間暴露臨時 ad-hoc 執行檔。
- `runUITest.command --build-only`：完成上述建置與簽署後結束，不開啟介面。

固定路徑與簽章身分有助於沿用螢幕錄製、輔助使用等已核准權限，但不保證 macOS 在升級、權限重設或其自身重新確認政策下永遠不再詢問。不修改 TCC 資料庫，也不自動授予權限。

Apple 公證涵蓋啟動器 APP；執行時另外建置的 bin 程式有 Developer ID 簽章，不因此自動納入啟動器的公證範圍。

參考：[Apple 程式碼簽章識別與授權](https://developer.apple.com/documentation/technotes/tn3127-inside-code-signing-requirements)、[Apple 公證流程](https://developer.apple.com/documentation/security/customizing-the-notarization-workflow)。
