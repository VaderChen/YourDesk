# Tailcat 備援連線可行性

研究日期：2026-09-11。此為原始碼與官方資料盤點，未修改 Client 的連線行為、加入相依套件或執行互連測試。

## 結論

可作為 YourDesk 的第二條連線路徑，值得先做隔離原型。產品操作可保留原本 ID、密碼與連線按鈕，由程式處理備援交換；目前無法保證穿透成功率、FPS 或延遲改善幅度。

Tailcat 提供 Go 函式庫及 CLI，使用 WireGuard、magicsock 與 DERP，不要求使用者登入 Tailscale 或安裝系統網路介面。它先透過 DERP 建立連線，再嘗試直接 UDP；穿透失敗時繼續經 DERP 中繼。因此它符合本次「原本穿不透時多一條路」的用途。[官方介紹](https://tailscale.com/blog/tailcat)

## YourDesk 現況與關鍵差異

本機程式 `internal/p2p/peer.go` 目前只有 Google／Cloudflare STUN，沒有 TURN，還會明確拒絕 relay candidate。畫面 DataChannel 不保證順序、不重傳；控制及剪貼簿使用可靠、有序通道。

`internal/signaling/direct.go` 的 TCP 47823 只交換配對資料。即使把這個埠放進 Tailcat，後續仍由 WebRTC 建立畫面資料通道，不能據此宣稱已解決穿透問題。備援必須包含實際資料傳輸，而非只轉送握手。

原有配對服務仍是自動找到對端的前提。若它本身連不上，單純把 Tailcat 加在 ICE 失敗後也無法自動取得對端地址；那是另一個服務發現問題，須另行設計。

## 整合路徑比較

| 路徑 | 評估 | 主要成本 |
| --- | --- | --- |
| Tailcat UDP 承載原有 WebRTC | 優先原型方向；盡量保留既有三種 DataChannel 與應用協定 | 需接入 Pion 的網路／UDP 介面、候選交換、MTU、對端綁定及關閉流程，不能當作一般 STUN 網址加入 |
| 獨立 Tailcat 傳輸後端 | 若 UDP 橋接不合適再考慮 | 抽出畫面、控制、剪貼簿傳輸介面；TCP 畫面可能累積過期影格，UDP 需自行分片、重組與處理遺失 |
| 原有 WebRTC 增加 TURN | 作為成本與成功率比較基準，預期改動較小 | 必須調整禁止 relay 的策略、加入憑證及中繼資源；同樣有維運與流量成本 |

TURN 是 WebRTC 的標準中繼途徑，並不需要 Tailcat，但不提供另一套 magicsock 穿透路徑。是否選它取代或搭配 Tailcat，應由後續實測決定。[WebRTC 官方 TURN 說明](https://webrtc.org/getting-started/turn-server)

研究時固定 Tailcat commit `91dc4979bd4ae88af6ae2c8bb549616de4bcaa5a`。其 API 包含 `Server.OnTCP`／`OnUDP`、`Client.DialTCPPort`／`DialUDPPort`；UDP 介面保留封包邊界，文件列出的最大 UDP payload 為 1232 bytes。不能直接把一張壓縮畫面視為單一 UDP 封包。地址可包含 PSK，必須視為秘密。[固定版本原始碼](https://github.com/tailscale/tailcat/blob/91dc4979bd4ae88af6ae2c8bb549616de4bcaa5a/tailcat.go)

## 建議連線流程

1. 原有配對與 WebRTC 優先；只在可歸因於傳輸的失敗／逾時後，且兩端均支援並允許備援時啟用 Tailcat。
2. 帳密錯誤、被控端拒絕、裝置離線等情況，不改用備援繞過驗證。
3. 新增獨立的能力與備援協商，綁定裝置、角色、工作階段及期限。現有 P2P 指令需要先連上，不能作為「尚未連上」時唯一的協商方式。
4. 既有 signaling HMAC 只有驗證與完整性，並不加密 Payload；不能把含 PSK 的 Tailcat 地址直接放進現有 offer、LOG 或站台匯出。先設計保密的交換方式，並在備援通道內保留 YourDesk 的對端驗證。
5. 每次連線只准一個傳輸後端取得輸入控制權。取消、STOP、退出與重連須回收另一條路徑，避免雙重 Host 或重複輸入。
6. 建立成功後沿用原顯示介面，工具列顯示直連／備援中繼；解釋放入泡泡。第一階段不承諾兩種傳輸後端之間無縫切換。

建議設定只加「備援連線」開關及統一的實驗標籤，測試階段預設關閉；開啟後自動處理備援，不要求使用者複製 Tailcat 地址、開 Terminal 或另建 Tailscale 帳號。

## 限制與工程工作

- **中繼成本與效能**：官方公開 DERP 有限速，未查到可承諾給 YourDesk 使用者的固定頻寬。持續桌面影像應評估自己的 DERP 或正式服務方案。DERP 也可能被網路封鎖，不能保證所有環境可用。[官方介紹](https://tailscale.com/blog/tailcat)、[DERP 不可用說明](https://tailscale.com/kb/1562/messages-client-no-derp-connection)
- **影像延遲**：經 DERP 的 TCP 承載可能出現隊首阻塞；即使上層用 UDP，也不代表中繼底層沒有此影響。需檢查頻寬優先級、畫面佇列、檔案傳輸與操作延遲互相影響的程度。[Tailscale hard NAT 說明](https://tailscale.com/docs/reference/troubleshooting/network-configuration/hard-nat-issues)
- **安全成熟度**：官方 SECURITY 明列外層仍為早期實驗工具，原始威脅模型偏向同一個人控制兩端。YourDesk 原型只使用必要的傳輸 API，不順便開啟 Tailcat 的 Shell、檔案分享、出口節點或任意轉送功能。[官方 SECURITY](https://github.com/tailscale/tailcat/blob/91dc4979bd4ae88af6ae2c8bb549616de4bcaa5a/SECURITY.md)
- **相依與建置**：固定版本要求 Go 1.27.1；本機工具鏈為 1.27.0，YourDesk go.mod 為 1.24.0。應先在獨立原型模組檢查工具鏈、相依版本、macOS／Windows 三種目標、體積與簽章流程，不直接更動主分支相依。[固定版本 go.mod](https://github.com/tailscale/tailcat/blob/91dc4979bd4ae88af6ae2c8bb549616de4bcaa5a/go.mod)
- **授權清單**：專案 LICENSE 為 BSD-3-Clause；正式打包前仍須整理實際引入的相依授權及聲明。[LICENSE](https://github.com/tailscale/tailcat/blob/91dc4979bd4ae88af6ae2c8bb549616de4bcaa5a/LICENSE)

若自架 DERP，應作為獨立服務規劃，不混入現有管理網頁的路由。這份研究沒有要求調整現有 Server 的 8080／443 分工。

## 後續原型的驗收範圍

先驗證單一受控工作階段，涵蓋正常直連、雙重 NAT、UDP 封鎖、DERP 不可用、錯誤密碼、取消及重連。記錄連線成功率、首張畫面時間、操作延遲、有效 FPS、CPU／記憶體與中繼流量；再確認鍵鼠、文字／圖片／檔案、STOP 和程序回收。重點是實際畫面與控制都能通過備援，而不只是握手或文字 ping 成功。

以上是後續工作清單，本次尚未執行原型或測試，亦未推送 GitHub 或更新 Release。
