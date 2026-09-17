# Windows 長時間連線後鍵鼠失效

更新日期：2026-09-18。狀態：已於 Windows 10 實機確認輸入桌面存取權限缺漏；修正後完整 APP 與長時間閒置情境仍待驗證。

## 回報與判斷

使用者回報遠端為 Windows、長時間不操作後，連線仍在但鍵盤與滑鼠無法使用；可能曾啟動螢幕保護程式。目前尚未確認失效時畫面是否仍更新、是否自動鎖定，以及重新連線是否恢復。

原實作直接在 Go 工作者呼叫 `SendInput`，未確認執行緒是否位於當前輸入桌面；桌面切換是待驗證原因，不能只依「連線仍在」就排除控制通道或顯示區域輸入狀態的問題。

## 已確認的桌面存取權限問題

2026-09-18 透過 MCP 檢查 Windows 10（10.0.19045.2364）連線：`controlEnabled=true`、`inputReady=true`，遠端 Shell 可執行。APP 送出滑鼠移動後游標位置不變；同帳號的獨立診斷程序直接呼叫 `SendInput` 則成功。

提交 `0daaad9` 加入桌面綁定時，`OpenInputDesktop` 僅要求 `DESKTOP_READOBJECTS | DESKTOP_WRITEOBJECTS`（`0x81`）。在診斷程序的獨立執行緒上重現相同流程，`SetThreadDesktop` 成功，但後續 `SendInput` 回傳 0、錯誤碼 5（`ERROR_ACCESS_DENIED`）。因此鍵盤與滑鼠共用的注入流程失效，獨立的畫面擷取與串流仍正常。

使用不改變游標位置的相對移動事件逐項比較桌面權限：

| 存取遮罩 | 綁定桌面 | SendInput |
| --- | --- | --- |
| `0x81`（原設定） | 成功 | 0，錯誤 5 |
| `0x83`、`0x85`、`0x89`、`0x91`、`0xC1`、`0x181` | 成功 | 0，錯誤 5 |
| `0xA1`（原設定加 `DESKTOP_JOURNALPLAYBACK`） | 成功 | 1，成功 |
| `0x1FF`（完整桌面存取權） | 成功 | 1，成功 |

修正僅補上 `DESKTOP_JOURNALPLAYBACK`（`0x20`），不要求完整桌面權限、不提升帳號權限。實機結果驗證了此權限缺漏，尚不代表已驗證更新後 APP 的端到端鍵鼠操作，也不能將先前所有閒置問題都歸因於此。

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
