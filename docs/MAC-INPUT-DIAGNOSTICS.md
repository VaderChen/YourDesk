# Mac 連線 Windows 的鍵鼠失效問題

2026-09-18 使用者更新後確認測試有效。原症狀為 Mac 筆電連上 Windows 後畫面正常，但鍵鼠無法控制；同一台 Windows 由桌上型 Mac 操作正常，Fn＋F12 無法恢復。

已確認 Windows Host 的輸入桌面權限缺少 `DESKTOP_JOURNALPLAYBACK`，補上後恢復鍵鼠注入。此修正需更新被控 Windows Host；詳見 [Windows 調查結果](WINDOWS-IDLE-INPUT.md)。本次確認不擴大為所有長時間閒置、鎖定或 GPU 停滯情境均已解決。

排查期間新增的 `viewer-input` 定期日誌及相關計數已移除；保留既有封包分析與正常錯誤回報。歷史日誌檔不因更新而刪除。
