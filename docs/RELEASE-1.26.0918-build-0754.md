# 1.26.0918 build 0754

修正遠端 Windows 畫面持續串流，但鍵盤與滑鼠無法操作的問題。Windows Host 綁定輸入桌面時補齊 `DESKTOP_JOURNALPLAYBACK` 權限，避免 `SendInput` 遭拒絕存取。使用者已確認更新後測試有效；這項修正需更新被控 Windows Host。

- 移除本次排查專用的 `viewer-input` 定期日誌及輸入計數；保留既有封包分析與正常錯誤回報。
- Windows 輸入使用固定 OS 執行緒並跟隨目前輸入桌面。遠端桌面連線期間防止自動休眠與關閉螢幕，斷線後解除；不取消系統鎖定或螢幕保護程式。
- Mac 游標改以 AppKit 取樣，工具列、遠端操作與裁切共用座標，改善觸控板位置未更新的情況。
- 補強心跳與控制工作者停滯偵測，以及 Windows GPU 讀回等待防護。新增連線計時、預設關閉的 15 分鐘閒置自動關閉，以及預設關閉的實驗性自動重連。自動重連不保證檔案續傳。

驗證範圍：使用者確認本次鍵鼠修正有效；Windows 10 API 比較亦確認原桌面權限遭拒、補齊後成功。長時間閒置、鎖定、GPU 或串流停滯仍需各自驗證，不宣稱所有斷線與停滯均已解決。詳見 [Windows 調查](WINDOWS-IDLE-INPUT.md) 與 [串流恢復說明](STREAM-RECOVERY.md)。

套件：macOS arm64 簽章／公證 DMG、Windows x64／arm64 安裝版、Windows x64 免安裝 ZIP、Linux x64／arm64 CLI Host、WinPE x64 實驗版。Android 不納入本次發行。
