# 登入前啟動：macOS／Windows 實驗實作

目前完成 macOS 與 Windows 服務、桌面代理與設定開關的程式實作。使用者已回報 Windows 服務成功啟動，初步實機測試可用；長時間穩定性與完整登入／登出、鎖定及輸入情境仍待觀察。此功能仍標示為實驗性，不能當成完整跨平台驗證已通過。

## 啟用條件與操作

在進階設定開啟「登入前啟動」。使用 macOS 14.4 以上、已簽署的 `YourDesk.app`，需完成系統管理員授權；未封裝的開發執行檔與 `clientUI.app` 不開放此操作。安裝時另外驗證 APP 的簽章信任鏈。

服務初次啟用時保存裝置 ID、連線伺服器、密碼、影像傳輸及 IP 直連選項；影像編碼與傳輸策略之後可直接同步修改。配對密碼保存在 root 專用設定檔，不寫入 plist、命令列或公開狀態。一般介面在服務模式不顯示目前使用者自己的密碼，避免誤認為它就是機器服務的密碼。

必須預先完成螢幕錄製與輔助使用授權。此實作不修改 TCC、不自動登入 OS，也不支援 FileVault 的開機前解鎖畫面。服務安裝成功只代表啟動工作已註冊，不代表畫面授權或 P2P 連線已成功；介面因此顯示「未登入連線服務模式」，不宣稱已連上。

## 程序與資料

- `/Library/LaunchDaemons/com.yourdesk.prelogin.broker.plist`：系統配對資料 broker。
- `/Library/LaunchAgents/com.yourdesk.prelogin.agent.plist`：Aqua／LoginWindow 圖形工作階段代理。
- `/Library/Application Support/YourDeskPrelogin/YourDesk.app`：受 root 保護的 APP 副本，服務不執行使用者可改寫的原始路徑。
- 同目錄 `config.json`：root 專用，權限 0600；`public.json` 僅提供裝置 ID。
- 同目錄 `broker.sock`：本機 UNIX socket；broker 以核心提供的 UID、PID、受保護執行檔路徑驗證代理。只授權目前主控台工作階段。
- 同目錄 `host.lock`：由實際 Host 持有的跨工作階段排他鎖，避免 broker 重啟或代理切換時重複註冊。

代理經私有 stdin 將密碼交給 Host，保留原本的父管線保護。broker 監控主控台 UID，切換時撤銷租約、回收已核對的子程序並預留清理時間，再授權新工作階段。程序身分比對使用路徑及啟動時間，避免僅依可重用的 PID。

目前採重新建立 Host／重新握手的工作階段交接，不保證登入切換時影像不間斷，也沒有加入 顯示區域 自動重連。主介面在服務安裝或啟用期間不再啟動自己的 Host；其他已更新的 UI 也會回收原本的 Host。舊版 UI 與自行啟動的舊版 Host 不在此交接管理範圍內。

## 停用、更新與失敗

關閉登入前啟動需管理員授權。停止 broker、回收代理後，確認 Host 排他鎖可取得才移除服務目錄；若尚有 Host 存活則保留目錄並回報錯誤，避免刪除鎖後產生第二個 Host。停用成功後，主介面恢復一般 Host 模式。

APP 自動安裝更新會等 APP 退出後停止登入前服務、更新主程式及受保護的服務副本，再恢復服務並開啟 APP；原本未啟用時不會安裝或啟用服務。Windows 新版服務沿用第一次管理員授權，後續 APP 內更新不再跳 UAC；舊服務首次遷移仍需一次授權。macOS 目前仍會取得管理員授權。配對設定及既有 SAS 原則保留，更新失敗會嘗試還原程式與服務備份。更新期間遠端連線會中斷，需重新連線。

macOS 自動更新需連續三次確認受保護 broker 可回覆 `health`，且目前工作階段的代理已交接有效 Host，才刪除舊版備份；不能以 launchd 工作註冊成功當成服務可用。健康檢查或新版啟動失敗時，還原 APP／服務副本，並以原登入使用者重新開啟原 APP；錯誤記錄保留。此檢查不代表螢幕授權、外部網路或 P2P 連線已驗證。

影像輸出編碼及傳輸策略可在服務啟用時直接修改：由目前主控台透過既有控制管道更新受保護的設定檔，服務自行重建 Host 套用，不需停用服務或重新取得管理員授權。既有被控連線會中斷，需重新連線。重複套用相同設定不再重啟；儲存失敗不改變服務設定，服務拒絕時還原本機偏好。舊服務需先透過 APP 更新取得此能力，不會以刪除限制的方式假裝套用成功。聲音編碼仍由連線協商，不受此限制。

修改配對密碼、IP 直連或 Tailcat 模式仍需先停用再啟用，避免介面與服務設定不同。手動安裝及 Windows ZIP 更新不經上述交接，仍需手動停用再啟用。

取消授權或安裝失敗會回報錯誤；安裝進度經狀態輪詢顯示，不受單次 HTTP 請求的短逾時限制。若清理失敗而保留服務目錄，介面維持服務模式以防重複啟動，應由管理員檢查後再次停用。

MCP 沒有隨此功能安裝成系統服務，也不會自動啟用；原本的 MCP 授權與預設關閉行為不變。

## 2026-09-12 工作階段與啟用檢查

- 啟用前確認呼叫端位於目前主控台的圖形工作階段，並查詢螢幕錄製與輔助使用權限。缺少權限就回報四語提示，不安裝服務、不修改 TCC。
- 代理以 `SessionGetInfo` 的圖形存取屬性與 `CGSessionCopyCurrentDictionary` 的主控台屬性確認自己可接手桌面；背景使用者的代理不取得配對資料或啟動 Host。
- 代理每秒核對圖形工作階段 ID 與前景狀態。離開前景或工作階段重建時關閉 broker 通道並回收 Host；由 launchd 在可用的 Aqua／LoginWindow 工作階段重新啟動代理。
- Host 等待排他鎖時也會重新檢查圖形工作階段，避免切換後還註冊舊桌面。既有 broker 的 UID、程序路徑與啟動時間檢查仍保留。
- 不以「是否已登入一般使用者」阻擋 LoginWindow，也不新增 root 命令列能力。登入前仍透過 macOS 原生登入畫面輸入帳號密碼，不代填或保存 OS 登入密碼。

啟用前查詢的是當前 APP 的權限；安裝後副本及 LoginWindow 的實際權限仍須實機確認。這些檢查不等於登入前擷取成功，也不保證跨工作階段無縫連線。若 macOS 在鎖定時將代理移出主控台，亦會進入上述回收／重試流程。

架構依據：[Apple DTS 的 LoginWindow／ScreenCaptureKit 說明](https://developer.apple.com/forums/thread/814152)。

## 驗證界線

### Windows 服務化

- `YourDeskPrelogin` 註冊為 SCM 的自動啟動服務，以 LocalSystem 執行。服務異常退出後由 SCM 重啟；網路連線沿用 Host 的重試流程。開機啟動不依賴使用者登入、Run 登錄項目或自動登入設定。
- 啟用透過 UAC 提升安裝程序，將目前 Client 複製到系統 Program Files 下的 `YourDeskPrelogin` 目錄。目錄僅 SYSTEM／管理員可寫；配對密碼的 `config.json` 使用明確 Windows ACL，僅 SYSTEM／管理員可讀寫。公開檔案僅提供裝置 ID。
- Session 0 的服務以自己的主權杖建立目前主控台 Session 的代理。跨 Session 不繼承匿名管線，改用隨機名稱、SYSTEM 專用的本機管線；密碼不寫入命令列。代理以 `OpenInputDesktop` 取得桌面名稱，在對應 `winsta0` 桌面建立 Host。
- 代理每秒觀察登入、登出、鎖定與解鎖造成的桌面變化；先回收舊 Host 再建立新 Host。服務亦監看主控台 Session 變更。兩層 Job Object 在父程序退出時終止子程序；另以服務目錄的檔案鎖避免重複 Host。桌面切換需要重新連線，不保證無縫串流。
- 本機控制管線接受狀態查詢、斷線、已驗證 Host 的 SAS 及官方服務更新交接，拒絕網路登入身分，不提供配對密碼或任意命令。服務 Host 沿用既有 SYSTEM 帳號不可開啟遠端終端機的限制。
- 停用經 UAC 停止 SCM 服務，等待停止後刪除註冊及專用副本。APP 自動更新會停止並更新服務副本；直接執行安裝程式或解除安裝仍要求先停用服務。一般使用者的站台資料不隨服務移除。
- 此次僅做交叉編譯與語法檢查，沒有 Windows 實機測試。仍須驗證 UAC 取消、首次開機、登入／登出、鎖定、快速使用者切換、服務重啟及移除。僅管理實體主控台 Session，不提供多個 RDP 工作階段；不支援 BitLocker 開機前解鎖或安全注意序列 Ctrl+Alt+Del 的合成。

架構依據：[Microsoft 互動式服務](https://learn.microsoft.com/en-us/windows/win32/services/interactive-services)、[CreateProcessAsUser](https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-createprocessasusera)、[SetThreadDesktop](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-setthreaddesktop)。

2026-09-12 修正簽章驗證參數：透過 `exec.CommandContext` 傳入 `-R` 時，規則字串必須是 `=anchor apple generic`。缺少 `=` 會被 `codesign` 當成檔案路徑，導致已簽署及公證的 APP 仍顯示簽章失敗。已對本機 `/Applications/YourDesk.app` 確認修正後的唯讀驗證成功；來源 APP 與安裝副本共用此驗證函式，保留深層、嚴格及 Apple 信任鏈檢查。參考 [Apple TN3127](https://developer.apple.com/documentation/technotes/tn3127-inside-code-signing-requirements)。

此次未安裝或啟用本機服務，未登出、重啟電腦，也未執行額外測試。實際支援範圍仍需在已授權實機檢查：開機未登入、鎖定、登出、快速切換使用者、TCC 拒絕、授權取消、Host 異常退出及停用回復。macOS 的架構依據見 [研究文件](UNATTENDED-ACCESS-RESEARCH.md)。

### 更新驗證範圍

已以隔離目錄驗證 macOS 更新成功、失敗還原與保留設定，並完成 Windows 交叉編譯及 NSIS 安裝器編譯。尚待實機驗證管理員授權、SCM／launchd 交接及更新後重新連線。Windows 舊服務首次遷移需以目前帳號提升權限；以另一帳號提供 UAC 憑證會停止更新，避免寫入錯誤的使用者安裝登錄。新版服務更新器的安裝器與 APP 始終保留原使用者身分。

## Windows 一次授權後的更新

- 第一次啟用登入前服務時，同一次 UAC 同時設定服務產生 SAS 的權限；後續 Ctrl+Alt+Del 由已安裝的服務處理。若管理原則後來撤銷 SAS 權限，不會在每次 APP 啟動時反覆要求 UAC，設定頁仍可由本機使用者明確選擇重新授權。
- APP 先核對控制管線確實屬於 SCM 登記的 LocalSystem 服務，再查詢更新能力。只有已驗證的舊服務不認識能力查詢時才走首次遷移；新版服務離線、下載失敗或拒絕請求，都直接回報原因，不退回 RunAs。
- 服務以 OS 提供的管線 PID 查出本機呼叫者 SID 與主控台 session。固定 worker 從 Program Files 的受保護服務副本啟動，另有跨程序檔案鎖；控制管線只授權原 SID／session，不能傳入執行命令、來源檔案或任意目的路徑。
- worker 自行向 `VaderChen/YourDesk` 的 GitHub Release API 取得同版同架構 `-service.zip`，必須有 GitHub SHA-256 與正確大小。正式版及已發布的測試版都可使用；不接受草稿、未知版本或降版。ZIP 只抽出四個服務檔案，檢查 manifest、PE 架構、重複檔名、連結及路徑越界。一般使用者下載的安裝器、腳本與 DLL 不會交給 SYSTEM 執行或直接複製成服務。
- 套件準備完成後才讓 APP 退出。worker 先停止及備份服務；一般使用者執行 NSIS，完成後通知 worker 安裝已驗證的服務檔案，啟動並核對服務版本／控制管線，再由原使用者重開 APP。此健康檢查不代表 P2P、螢幕授權或外部網路都已通過。
- 安裝、啟動或健康檢查失敗會嘗試還原；worker 不隨 SCM 的停止而結束，交接逾時亦會回復服務。配對設定、SAS 原則與安裝目錄的非管理檔案不受影響。失敗記錄留在使用者更新暫存目錄；無法回復的服務備份保留於 `Program Files/YourDeskPrelogin/updates`，不宣稱可防止斷電造成的所有部分更新。
- Release 必須同時上傳每個 Windows 架構的 `-setup.exe` 及 `-service.zip`；缺少附件或雜湊時，APP 會留在原版本並顯示錯誤。手動安裝、停用／移除服務及變更配對／網路設定仍需管理員授權；影像編碼與傳輸策略透過既有服務同步，不重新授權。

本輪完成 Windows x64／ARM64 交叉編譯、套件拒絕條件、交易失敗回復與隔離更新腳本 Smoke。尚未在 Windows 實機跑完「首次遷移 → 下一版免 UAC 更新」；不把編譯及替身測試當成實機驗證。

## 編碼設定同步驗證

完成設定持久化、只更新編碼欄位、重複請求、非法編碼、服務拒絕及本機儲存失敗的 Smoke，並檢查啟用服務時的設定頁操作。macOS 控制管道沿用受保護 helper 的 UID／PID／路徑驗證；Windows 核對 SCM 管線及目前主控台工作階段。這些檢查不等同於已在實機重啟 launchd／SCM 服務；本次不改動正在使用的系統服務。
