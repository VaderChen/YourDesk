# 開機與未登入遠端連線研究

研究日期：2026-09-11。本文為可行性與架構提案，尚未實作、安裝服務或進行登入畫面實測。

## 結論與範圍

Windows 與 macOS 都有支援 OS 登入畫面遠端連線的技術途徑。核心是系統服務維持 Host，再由目前圖形工作階段的專用程序擷取畫面與注入輸入。僅設定登入後自動啟動 APP 不足以提供此功能。

| 遠端狀態 | 預計能力與邊界 |
| --- | --- |
| 已登入、APP 關閉 | 系統服務持續提供被控功能；須另有明確停用入口 |
| 鎖定畫面 | 保留服務，處理安全桌面及權限；與登出分別驗證 |
| OS 已啟動、無人登入 | Windows 目標工作階段代理；Mac LoginWindow Agent |
| 登出／切換使用者 | 更換桌面代理，機器 Host 身分不變 |
| OS 重開機 | 原連線一定中斷；服務啟動、網路恢復後重新握手 |
| FileVault／BitLocker 開機前解鎖 | 一般 YourDesk Host 尚不能運作，不能視為 OS 登入畫面 |
| 睡眠／完全關機 | 另屬喚醒功能，取決於硬體、韌體、電源與網路 |

## YourDesk 現況

`internal/clientui/server.go` 的 `keepHostRunning` 由主介面啟動 Host，傳入 `-parent-stdin`。`cmd/client/main.go` 在父程序管線關閉後取消 Host，五秒後仍未結束則退出。這是避免孤兒 Host 的既有設計；不能為了開機連線而單純移除父子程序保護。

Mac 擷取已使用 ScreenCaptureKit（`internal/desktop/capture_stream_darwin.m`），Windows 輸入使用 SendInput（`internal/input/input_windows.go`）。現有元件可部分沿用，但不能據此認定已具備未登入桌面的執行環境及權限。

## Windows 機制

由 SCM 管理常駐服務與故障重啟。服務位於 Session 0，不能直接把目前 Host 當成互動服務顯示桌面；Microsoft 建議服務與工作階段內程序分離，以受 ACL 保護的 IPC 溝通。[Interactive Services](https://learn.microsoft.com/en-us/windows/win32/services/interactive-services)

YourDesk 可由服務監控主控台及登入狀態，將擷取／輸入代理啟動於正確的互動工作階段。尚未登入時不能依賴取得已登入使用者 token 的路徑；安全桌面代理需要獨立處理其受限的系統權限。一般使用者介面仍以一般權限執行。工作階段切換時回收舊代理與 GPU 資源，重新建立擷取器。

DXGI 明確指出安全桌面存取需要 LOCAL_SYSTEM，且桌面模式、工作階段變化可能要求重建 duplication。這是權限需求，並不表示整個 GUI 或所有網路處理都應提升為 SYSTEM。[DuplicateOutput](https://learn.microsoft.com/en-us/windows/win32/api/dxgi1_2/nf-dxgi1_2-idxgioutput1-duplicateoutput)

Ctrl+Alt+Del 需要專門的 SendSAS 路徑與相應政策條件，不能當成三個普通 SendInput 按鍵。UAC、安全桌面、鎖定與登出各自列入驗證，不以停用 UAC 解決。[SendSAS](https://learn.microsoft.com/en-us/windows/win32/api/sas/nf-sas-sendsas)

## macOS 機制

Apple DTS 工程師明確建議：Daemon 管理全域網路，Agent 處理畫面與鍵鼠，以 XPC 等 IPC 溝通；Agent 涵蓋 Aqua 與 LoginWindow。他也確認 macOS 14.4 起修正了 ScreenCaptureKit 在登入前環境的問題，舊版需其他擷取技術。[Apple 工程師回覆](https://developer.apple.com/forums/thread/814152)

實際 launchd plist 鍵名為 `LimitLoadToSessionType`（本機 `man 5 launchd.plist` 已核對），預計設定 Aqua、LoginWindow。此鍵適用 Agent，不適用 Daemon。登入後更換 Agent，Daemon 保留機器身分；不能只把 GUI 程式放進 LaunchDaemon。[Apple launchd 文件](https://developer.apple.com/library/archive/documentation/MacOSX/Conceptual/BPSystemStartup/Chapters/CreatingLaunchdJobs.html)

建議首階段以 macOS 14.4 以上為實驗範圍。安裝與螢幕錄製、輔助使用授權必須事先完成；root 不等於免除 TCC。簽章、安裝位置、Agent 身分與更新後授權保留需要實測。論壇提問者對特殊 entitlement 的猜測不是 Apple 給出的必要條件，不應據此承諾或申請能力。

## 開機前磁碟解鎖與喚醒

Apple 文件另有特例：Apple silicon、macOS 26 以上，預先啟用 Remote Login 且有網路時，可在重新啟動後透過 SSH 解鎖 FileVault。這是 Apple 的獨立機制，不代表 YourDesk P2P、MCP 或自訂 Daemon 已在解鎖前可用，也不能延伸保證所有冷開機情境。[FileVault 管理](https://support.apple.com/guide/security/managing-filevault-sec8447f5049/web)

Windows BitLocker 開機前 PIN／復原畫面在 OS 服務啟動前；一般 Host 無法操作。TPM 自動解鎖與要求開機 PIN 是不同部署條件。[BitLocker FAQ](https://learn.microsoft.com/en-us/windows/security/operating-system-security/data-protection/bitlocker/faq)

完全關機時 Host 不會接收指令。Wake-on-LAN 需硬體與電源設定支援；跨網路可規劃由同網段已授權且在線的節點代送喚醒封包，不能假設中央伺服器可穿透 NAT 直接喚醒。Windows Fast Startup／關機狀態的 WoL 有明確限制；Mac 的網路喚醒主要是睡眠情境，不能承諾通用遠端冷開機。[Windows WoL](https://learn.microsoft.com/en-us/troubleshoot/windows-client/setup-upgrade-and-drivers/wake-on-lan-feature)、[Mac 網路喚醒](https://support.apple.com/en-ie/guide/mac-help/-mchlp2995/mac)

## 建議的共用架構與功能

以下是 YourDesk 的設計提案，並非已具備的能力。

1. 系統服務是唯一 Host 註冊擁有者。UI 與桌面代理透過本機 IPC 接入，避免重新引入重複 Host；現行一般模式保留，啟用服務模式時明確交接所有權。
2. 機器 ID 與無人值守認證採機器範圍儲存及存取限制，不能依賴某個使用者登入後才可開啟的設定。遷移時保留既有 ID，明確處理密碼所有權。
3. 增加「開機後允許遠端連線」設定，預設關閉，由本機管理員啟用。關閉主視窗與停止被控服務分開表達，Tray／設定提供停用入口。
4. 本機 IPC 僅接受已授權元件，命令採固定功能集合；桌面代理權限限於必要工作。MCP 沿用既有驗證及允許範圍，不能因服務模式自動取得額外系統控制權。
5. 顯示在線但無桌面、登入畫面、鎖定、桌面切換中等狀態；沒有可靠訊號時標示離線或原因未知，不能猜測對方正在 FileVault。
6. 更新器協調服務停止、代理回收、檔案替換與服務恢復，失敗可回復。重新連線須重做認證，不沿用舊 SDP；記錄工作階段世代、狀態與時間，避免記錄密碼及輸入內容。

## 開源參考與實作順序

RustDesk 的 Windows 平台程式有服務控制處理，Mac 平台程式處理 LoginWindow 與多 GUI 工作階段，可作為生命週期案例。此次只檢視部分平台程式，未完整稽核擷取、權限與所有版本行為，也未複製程式碼。[Windows 原始碼](https://github.com/rustdesk/rustdesk/blob/master/src/platform/windows.rs)、[macOS 原始碼](https://github.com/rustdesk/rustdesk/blob/master/src/platform/macos.rs)

先抽離 Host 的服務生命週期與唯一擁有者，再分別驗證 Windows 登入畫面／UAC 與 Mac LoginWindow／Aqua 交接，最後才接入更新重啟與選配喚醒。發布前需實機覆蓋首次開機未登入、鎖定、登出、切換使用者、網路延遲、服務故障與更新失敗；這些是未來驗收條件，本次沒有執行測試。
