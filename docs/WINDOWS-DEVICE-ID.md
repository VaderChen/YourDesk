# Windows 裝置 ID

Windows 不再使用 Win32_Processor.ProcessorId 產生裝置 ID：此值描述 CPU 特徵，不保證每台電腦唯一，同型號電腦可能撞號。

## 新生成順序

1. 讀取 HKLM 的 `SOFTWARE\Microsoft\Cryptography\MachineGuid`，固定使用 64-bit registry view，讓不同帳號、程式架構與 Host 子程序使用一致來源。
2. 無法取得有效 MachineGuid 時，查詢 `Win32_ComputerSystemProduct.UUID`（SMBIOS 系統 UUID）；查詢最多等待 5 秒。
3. 兩者皆無效則明確回報錯誤，不改用 CPU 資訊、網卡或共用預設 ID。

GUID 統一大小寫與格式，拒絕格式錯誤、全零及全 F。識別值加上各自的來源標記後做 SHA-256，保留既有 `YD-XXXX-XXXX-XXXX-XXXX-XXXX` 外觀；不傳送或記錄原始 GUID。

## 更新影響

- Windows 自動產生的裝置 ID 會改變；須以設定頁的新 ID 更新已儲存站台。舊 ID 不建立自動別名，避免仍連到撞號電腦。
- 不主動重設連線密碼或既有設定；手動指定的 room 仍優先。
- macOS 與其他平台維持原本生成方式。
- MachineGuid 代表 Windows 安裝。一般重新開機與 YourDesk 更新不會改變它；重灌 Windows、修改登錄識別值或主要／備援來源可用性改變，可能改變 ID。
- 複製 Windows 映像或虛擬機若連識別資料一併複製，仍可能撞號。本次不宣稱解決所有複製映像情境，也不新增 Server 強制停止 Host 功能。

編譯確認涵蓋 Windows amd64、Windows arm64 與 macOS 的 deviceid 套件；Windows 實機的新 ID 與重新連線仍待確認。
