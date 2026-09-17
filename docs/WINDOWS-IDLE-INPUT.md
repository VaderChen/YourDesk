# Windows 長時間連線後鍵鼠失效

更新日期：2026-09-17。狀態：候選修正，待 Windows 實機確認。

## 回報與判斷

使用者回報遠端為 Windows、長時間不操作後，連線仍在但鍵盤與滑鼠無法使用；可能曾啟動螢幕保護程式。目前尚未確認失效時畫面是否仍更新、是否自動鎖定，以及重新連線是否恢復。

原實作直接在 Go 工作者呼叫 `SendInput`，未確認執行緒是否位於當前輸入桌面；桌面切換是待驗證原因，不能只依「連線仍在」就排除控制通道或顯示區域輸入狀態的問題。

## 候選處理

- Windows 鍵鼠統一交由不建立視窗／掛鉤的固定 OS 執行緒。每次操作先取得目前輸入桌面；桌面改變時重新綁定，再執行滑鼠、一般按鍵或原始掃描碼注入。相同桌面不重複綁定，查詢 handle 用完即釋放。
- 桌面暫時無法存取時回報錯誤，不向舊桌面注入，也不把失敗事件保存到解鎖後重播。可存取後，下一筆新操作重新嘗試。`SendInput` 未接受事件但沒有錯誤碼時，也明確回報失敗。
- 僅在 Windows 已建立的遠端桌面連線期間要求系統與顯示器維持啟用。斷線或工作階段取消後解除，不因主程式常駐而持續防休眠；不改寫系統電源設定。
- 防休眠要求與解除在同一個專用 OS 執行緒執行，結束時退役執行緒。Linux CLI 與其他平台行為不變。

防自動休眠不等於停用螢幕保護程式或鎖定。此修正不關閉這些功能，也不提升權限；安全桌面或較高完整性層級仍受 Windows 存取控制限制。

## 驗證範圍

本機 Smoke 模擬桌面切換、取得／綁定桌面遭拒後恢復，以及持續在同桌面操作時的 handle 交接，確認拒絕期間不注入、恢復後不重播。另檢查 Windows x64／arm64 編譯。這些不取代實際 Windows 的螢幕保護、鎖定、電源管理與長時間連線測試。

實機測試需更新被控的 Windows Host。確認連線閒置期間不自動休眠／關閉螢幕，並在螢幕保護及鎖定後恢復桌面，測試鍵鼠是否可繼續使用；斷線後確認原有電源策略恢復。

## 官方依據

- [OpenInputDesktop](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-openinputdesktop)：取得當前接收輸入的桌面。
- [SetThreadDesktop](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-setthreaddesktop)：綁定呼叫執行緒，已有視窗或掛鉤會限制切換。
- [SendInput](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-sendinput)：回傳已接受事件數，並受 UIPI 限制。
- [SetThreadExecutionState](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-setthreadexecutionstate)：可防止自動休眠／關閉螢幕，但不阻止螢幕保護程式，也不阻止使用者主動休眠。
