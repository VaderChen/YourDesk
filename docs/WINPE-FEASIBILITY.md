# YourDesk 在 WinPE 的可行性研究

後續已有實驗性 Host 實作，見 [實作狀態](WINPE-IMPLEMENTATION.md)。以下保留研究當時的評估；實機相容性仍未驗證。

研究日期：2026-09-11。範圍為讓已啟動 WinPE 的電腦接受 YourDesk 遠端救援；不是 BIOS 遙控，也不是一般 Windows 登入前服務。本次只有官方文件與本機原始碼盤點，未建立 PE 映像、修改程式或執行互連測試。

## 結論

**值得做精簡 Host 原型，但不能直接宣稱目前 Windows 安裝包可在 WinPE 運作。** 建議第一階段限定 Windows 11 系列 WinPE x64、有線網路、單螢幕、GDI 擷取與軟體 JPEG，近端仍使用正常 macOS / Windows 的 YourDesk Remote。

WinPE 可執行 Win32 部署與修復工具；Microsoft DaRT 也有從復原映像啟動後接受遠端操作的先例。這支持「救援環境可提供遠端操作」的方向，並不證明 YourDesk 的 Go、WebRTC 或 Tailcat 已相容。[WinPE 概覽](https://learn.microsoft.com/en-us/windows-hardware/manufacture/desktop/winpe-intro?view=windows-11)、[DaRT 遠端復原](https://learn.microsoft.com/en-us/microsoft-desktop-optimization-pack/dart-v10/how-to-recover-remote-computers-by-using-the-dart-recovery-image-dart-10)

## 建議的產品形態

```text
USB / ISO / PXE 啟動 WinPE
        ↓
wpeinit → 網卡 / IP / DNS 就緒
        ↓
YourDesk PE Host：ID、一次性密碼、停止連線按鈕
        ↓
既有配對驗證 + WebRTC + 共用虛擬傳輸層
        ↓
正常 Mac / Windows 的 YourDesk Remote
```

使用便攜 EXE 與精簡原生 Win32 視窗，或先用主控台，不依賴 Explorer、Tray、WebView2 或安裝程式。啟動腳本等待網路初始化後才啟動 Host；重啟 PE 後重新建立工作階段。這是設計建議，尚未實作。

Microsoft 提供 Startnet.cmd / Winpeshl.ini 的啟動方式，wpeinit 負責 PnP 與網路初始化；其程式設計文件也提醒 PE 只有部分 Windows API，並要求注意原生 runtime 相依。Go 並非該文件保證的執行環境；需獨立驗證 Go runtime 能啟動，而非只看 Windows 編譯成功。[WinPE 應用程式](https://learn.microsoft.com/en-us/windows-hardware/manufacture/desktop/winpe-create-apps?view=windows-11)、[Go 最低系統需求](https://go.dev/wiki/MinimumRequirements)

## 現有程式的接入點

| 模組 | 本機證據 | PE 方向與待確認項目 |
| --- | --- | --- |
| Host 入口 | cmd/client/main.go 同時匯入 clientui 與 Host 功能 | 抽出共用 Host 核心，建立獨立 PE 入口；單純不加 -ui 不代表二進位已排除 UI 相依 |
| 畫面擷取 | internal/desktop/capture_stream_windows.go 已有 DXGI → GDI 路徑 | 第一版直接使用 GDI，實機確認基本顯示驅動、解析度及 BitBlt / DIB 可用性 |
| 編碼 | internal/desktop/capture.go 使用 Go image/jpeg；internal/winmedia 連結 Media Foundation / D3D11 | PE 建置排除硬體編碼相依，先選軟體 JPEG；不能假設缺 DLL 時仍能進入程式內的降級分支 |
| 鍵鼠 | internal/input/input_windows.go 使用 user32 SendInput | 可作為原型起點，需核對 PE 的輸入桌面、鍵盤配置與滑鼠座標；未驗證 |
| 剪貼簿 | internal/clipboard/native_windows.go 使用 Win32 clipboard，檔案延遲傳送另有 OLE / Shell 整合 | 先驗證文字；圖片及 Explorer 式檔案複製另列能力，不能沿用正常 Windows 的支援宣告 |
| 傳輸 | internal/p2p 與 internal/peertransport 已分離 | 可沿用原生 P2P、Tailcat 與後續 Relay 架構；需要 PE 網路 API 的實際驗證 |
| 裝置 ID | internal/deviceid/identity_windows.go 優先 MachineGuid，失敗改查 PowerShell / CIM UUID | 不應直接採用救援映像的 MachineGuid，也不宜強制要求 PowerShell |
| TLS | internal/security/tls_client.go 使用系統信任、內嵌公開憑證及可選 CA | 配對服務與 Tailcat 公開 HTTPS / DERP 的信任來源要分別驗證，並核對時鐘；不能以停用驗證解決 |

GDI 與 SendInput 的可行性是基於現有程式與 Win32 介面的工程推論，不是已在 PE 證實。

## 必須先處理的障礙

### 完整 UI 與 Windows 多媒體相依

WebView2 官方回饋專案的已採納回答於 2024 年指出 WinPE 尚不在正式支援範圍；後續討論仍有啟動問題。此次未找到可據以保證目前 WebView2 / WinPE 相容的正式支援聲明。WinPE-HTA 是另一種引擎，不能直接取代目前 HTML UI 的 WebView2。[WebView2 官方專案討論](https://github.com/MicrosoftEdge/WebView2Feedback/discussions/4293)、[WinPE 選用元件](https://learn.microsoft.com/en-us/windows-hardware/manufacture/desktop/winpe-add-packages--optional-components-reference?view=windows-11)

目前 winmedia 的 cgo LDFLAGS 包含 mfplat、mf、mfreadwrite、d3d11、dxgi 等。這帶來 PE 載入相依風險；僅設定 software-jpeg 不一定足以排除。應以建置邊界隔離，而非從完整 Windows 零散複製系統 DLL。第一個關卡是檢查 PE 專用 EXE 的匯入表及啟動結果。

### 裝置識別與暫存狀態

同一 boot.wim 部署到多台電腦時，登錄資料可能來自同一映像；其 MachineGuid 不應被假設為每台機器唯一。若照現有順序取值，有再次發生 Host ID 重複的風險。這是程式路徑分析，尚未量測不同 PE 映像實際的 MachineGuid。

建議救援模式預設建立「每次開機的暫時 ID 與密碼」，生命週期內保持一致；若需要跨次開機辨識，再增加經驗證的 SMBIOS 身分或受控的每裝置持久設定。避免把所有機器共用的 ID / 密碼直接做進映像。不要自動讀取離線 Windows 的身分檔來冒用其正常 Host 工作階段。

WinPE 的暫存登錄變更會隨重啟消失；需要保存的記錄或設定須明確選擇 USB / 外部可寫位置，亦不可假設離線 Windows 一定位於 C 槽。[WinPE 限制](https://learn.microsoft.com/en-us/windows-hardware/manufacture/desktop/winpe-intro?view=windows-11)、[DaRT 磁碟機代號說明](https://learn.microsoft.com/en-us/microsoft-desktop-optimization-pack/dart-v10/how-to-recover-remote-computers-by-using-the-dart-recovery-image-dart-10)

### 網路與 Tailcat

標準 WinPE 官方文件明列一般 Wi-Fi 功能不受支援；第一版應以有線網路為基準，必要時注入網卡驅動。802.1X 等環境另有元件需求。第三方 PE 的擴充不能當作標準相容性依據。[WinPE 網路](https://learn.microsoft.com/en-us/windows-hardware/manufacture/desktop/winpe-network-drivers-initializing-and-adding-drivers?view=windows-11)

Tailcat 固定版本在 createEngine 使用 userspace engine / netstack，與目前 PacketConn 虛擬層相容，設計上不需另外安裝虛擬網卡供上層使用。但它仍依賴 Windows 的網路列舉、DNS、socket、TLS 與其他 runtime API；userspace 不等於已支援 WinPE。需分開驗證原生 WebRTC、Tailcat 直連與 DERP。[Tailcat 固定版本原始碼](https://github.com/tailscale/tailcat/blob/91dc4979bd4ae88af6ae2c8bb549616de4bcaa5a/tailcat.go)

Tailcat 的公開 HTTPS 連線不會自動沿用 YourDesk TLSHTTPClient 的額外 CA 設定；PE 的憑證存放區與系統時間是獨立檢查項目。PE 沒有可用網卡、DNS 或對外連線時，Tailcat 也無法補足。

## Tailcat 能力檢查與停用規則

使用者確認：WinPE 環境不支援 Tailcat 時，開關關閉並灰色 DISABLE，原因統一放進說明泡泡。尚未完成相容性確認的 PE 組合先視為不可用，不把 Windows 編譯通過當作支援證據。

同一份能力結果應供 UI、偏好設定 API、CLI 與 P2P 協商共用。不可用時不宣告 Tailcat capability，拒絕遠端的 Tailcat 要求，也不得因讀入既有 enabled 設定而啟動。這項檢查必須涵蓋單端開啟的自動配合流程，不能只把本機開關灰化。原因應區分缺少必要 API / 元件、未支援的 PE 版本或架構等。

網路暫時中斷、DNS 或 DERP 不可達屬於連線失敗，不等同作業環境不支援，不應永久停用開關。兩端可共同使用原生 P2P，但在明確要求 Tailcat 而不可用時應回報原因；依目前設計不靜默降回原生傳輸。

以上為 WinPE 原型的實作需求，本次研究尚未在正式 Client 加入 PE 偵測或能力停用程式。

## 原型順序與通過條件（尚未執行）

1. 建立獨立 Host 建置入口，排除 UI、更新安裝、Media Foundation 與不必要的 Shell 整合；在乾淨 WinPE x64 確認 EXE 可啟動、不缺 DLL。
2. 驗證 GDI 取得實際 PE 畫面、軟體 JPEG 編碼與 SendInput 操作。從低 FPS / 單螢幕開始，避免把高效能當第一關。
3. 加上暫時 ID、一次性密碼與原生 P2P；由一般 YourDesk Remote 連入，檢查驗證、首張畫面、STOP、斷線與重新連線。
4. 使用同一虛擬層驗證 Tailcat：分別測 UDP 可通、UDP 受阻 / DERP 路徑；必須看到實際畫面與鍵鼠，不能只驗證握手。
5. 再評估文字 / 圖片 / 檔案、無人值守啟動、ARM64、多網卡及磁碟救援操作。每項功能回報能力，不把正常 Windows 的支援清單原樣套用。

MCP / 遠端 Shell 應另外設定救援環境授權，不能因 PE 缺一般登入流程便自動放行。既有 remotedata 的 Geteuid 判斷也不能視為 Windows LocalSystem 的完整權限判斷；需要 Windows token 檢查與獨立政策。這是後續設計項目，本次不開啟任何新能力。

## 使用邊界

WinPE 適用於部署與修復，不適合作為長期常駐的一般桌面系統；官方亦不支援內建 Remote Desktop 與 MSI。這不等於自製救援通道一定不可能，但應以部署／復原工具定位開發。須先有 USB、ISO、PXE 或既有復原分割區能啟動 PE，才能連入；YourDesk 本身不能接管尚未啟動 PE 的 BIOS / UEFI。[WinPE 概覽與限制](https://learn.microsoft.com/en-us/windows-hardware/manufacture/desktop/winpe-intro?view=windows-11)

第一版建議提供可放入使用者自行建立之 ADK WinPE 的 Host 與啟動腳本，暫不在研究階段承諾分發完整 PE 映像。此研究不修改既有 Client、不推送 GitHub、不發布 Release。
