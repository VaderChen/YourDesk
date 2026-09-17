# 待發行修正

本輪修改已整理至 [1.26.0915 build 1524](RELEASE-1.26.0915-build-1524.md)。後續變更由此繼續記錄。

## Windows 串流正常但鍵鼠失效

- 補上輸入桌面的 `DESKTOP_JOURNALPLAYBACK` 存取權，修正綁定桌面成功後 `SendInput` 仍遭拒絕存取，導致鍵盤與滑鼠同時失效的問題。
- Windows 10 實機 API 比較已確認原權限失敗、補上該權限後成功；需更新 Windows Host，完整 APP 更新後的端到端操作仍待驗證。詳見 [診斷結果](WINDOWS-IDLE-INPUT.md)。

## Windows 閒置連線輸入恢復（待實機驗證）

- 鍵鼠在固定 OS 執行緒注入，送出前確認並跟隨目前輸入桌面；桌面暫時不可存取時不重播失敗事件。
- 遠端桌面連線建立後暫時防止 Windows 自動休眠／關閉螢幕，斷線後解除；不關閉螢幕保護程式或系統鎖定。
- 只需更新被控 Windows Host。模擬 Smoke 與 Windows x64／arm64 編譯檢查不代表已完成長時間 Windows 實機驗證。詳見 [調查與驗證範圍](WINDOWS-IDLE-INPUT.md)。
