# Tailcat 虛擬傳輸整合（實驗性）

## 操作方式

兩端皆需使用包含本次修改的新版程式，只需任一端於「設定 → 網路安全」開啟 Tailcat 模式。預設關閉；切換後本機 Host 重新啟動，被控連線中斷，對外連線須重新連線才改用新模式。已啟用未登入服務時，須先停用服務再修改設定，重新啟用時會保存新的模式。

兩端皆關閉時維持原生 WebRTC P2P；任一端開啟時，畫面、控制與剪貼簿全部透過 Tailcat UDP。另一端通過密碼驗證後自動配合，但不修改其偏好設定。背景診斷、快速連線、站台連線與 MCP 建立的連線共用設定。對端版本不支援所需協商或模式時回報錯誤，不靜默改用其他路徑。

CLI Host / Remote 可使用 `-transport tailcat`。這一版是明確選用模式，尚未實作原生連線失敗後自動切換。Tailcat 內部的 UDP 直連與 DERP 中繼選擇由上游處理，不能保證穿透成功率或中繼畫面速度。

## 單端協商

| Host 開關 | Viewer 開關 | 本次傳輸 |
| --- | --- | --- |
| 關 | 關 | 原生 WebRTC P2P |
| 開 | 關 | Tailcat；Viewer 接受已驗證的 Host offer |
| 關 | 開 | Viewer 提出切換要求，Host 關閉原生 Peer 後發出新 Tailcat offer |
| 開 | 開 | Tailcat |

原生 offer 附帶協商版本與支援模式。Viewer 要求綁定原 offer 的 SHA-256，並由既有 signaling HMAC 保護。升級訊息使用既有 KindError envelope 的 transportRequest 欄位，避免更動 Server 路由，也不會被誤認為最終 answer。Host 僅接受對應當前原生 offer 且已註冊的模式，最多升級一次；Viewer 忽略排隊中的原 offer，75 秒內等待新 offer，仍受外層連線逾時約束。

不另開 Host 程序或平行生效的控制通道。設定頁顯示本機偏好，連線分析顯示工作階段實際模式。關閉本機開關表示不主動要求 Tailcat，並非拒絕已驗證對端要求的 Tailcat 通道。

## 共用虛擬層

```text
畫面 / 控制 / 剪貼簿 / P2P 指令
                 ↓
      WebRTC DataChannel / DTLS / ICE
                 ↓
          peertransport.Open
          ├─ 原生：Pion 系統 UDP
          ├─ Tailcat：虛擬 PacketConn → Tailcat UDP
          └─ 未來 Relay：實作 Backend / Link 並註冊新模式
```

`internal/peertransport` 提供 `Mode`、`Backend`、`Link` 與註冊介面。傳輸後端不處理滑鼠、畫面或檔案；只管理單一工作階段的封包通道與配對資料。原生模式回傳 nil，保留 Pion 的既有 STUN / ICE；其他模式透過 Pion UDP mux 使用私有 PacketConn。

新增具備 datagram 語意的 Relay 後端時，實作 Host、Viewer、PacketConn、Offer 與可重複呼叫的 Close，再擴充模式選擇即可沿用 WebRTC。若未來採 TURN 等 Pion 自有的 Relay，應在 `newTransportPC` 增加相對應配置策略，不能直接把 TURN URL 當成 PacketConn 後端。

Tailcat 實作使用每個 Link 私有的 127.0.0.2 / 127.0.0.3 虛擬端點，不在這些位址建立 OS socket，也不修改系統路由。47824 為 Tailcat 虛擬 UDP 埠，與 TCP 47823 的既有 IP 直連握手、MCP 12345 分開。WebRTC SDP 只宣告虛擬端點；UDP mux 的所有封包皆經 Tailcat，不會另開系統 UDP 傳送這些候選資料。

Tailcat 封包每片最多 1112 bytes，小於上游 1232-byte 限制。重組限制為 32 組、每包 65535 bytes、2 秒到期；接收佇列 32 包，滿載丟棄。可靠重傳與畫面不重傳仍交由原來的 WebRTC 通道策略處理。DERP 底層可能產生額外延遲，並不保證 UDP 上層語意可消除其阻塞。

## 驗證與生命週期

- 沿用 signaling HMAC、配對密碼與 WebRTC DTLS。Tailcat 位址含 PSK，使用 AES-256-GCM 加密後加入 offer，金鑰以現有 secret 做 HMAC-SHA256 的用途分離衍生。
- 加密附加資料綁定 room、Host 角色、直連 TLS binding 與完整 SDP；SDP 包含該次工作階段的 ICE 憑證。外層 envelope 仍有簽章及時效驗證。
- Tailcat 使用每次新建的暫時身分與 PSK；不保存、不列印明文位址。只允許指定 UDP 埠，不開放 Shell、檔案伺服器、TCP、任意轉送或出口節點。
- Tailcat 初始化 / 撥號限制 30 秒。Host 上游 Start 沒有 context，取消後會在初始化返回時回收；尚不能保證立刻中止上游內部的初始化操作。
- 同一 Peer 只使用一條 Link，沿用既有 STOP、關閉及重連流程回收。模式變更不偷偷替換已建立的對外連線。
- 連線分析回報工作階段的實際模式；顯示 Tailcat 不代表已確認其底層當時使用 UDP 直連或 DERP。

## 建置與驗證範圍

固定上游 `github.com/tailscale/tailcat v0.6.1-0.20260909154426-91dc4979bd4a`；Go 最低版本提升至 1.27.1，並更新必要的間接相依。新增相依使執行檔體積增加，正式發佈仍須納入依賴授權盤點。

已通過 macOS ARM64 Client / Remote 與 Windows x64 / ARM64 共用傳輸套件的編譯。建置輸出放在 `/tmp/yourdesk-tailcat-build/`，不含簽章 App 或安裝包。本次只做語法及編譯確認，沒有實機 Tailcat / DERP 互連、畫面、剪貼簿、取消、未登入 OS 或效能測試。不可將編譯成功視為已驗證穿透成功。尚未推送 GitHub、更新 Release 或替換正在執行的 App。
