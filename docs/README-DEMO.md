# README 操作動畫

四語 README 共用 `images/yourdesk-demo.gif`，以繁體中文實際 Client UI 展示群組切換、搜尋、新增站台、明暗外觀、遠端 ID 輸入與站台連線密碼視窗。

## 錄製範圍

- 直接載入 `internal/clientui/web` 的 HTML、CSS、JavaScript，不另畫一套介面。
- 使用虛構名稱、裝置 ID 與記憶體中的站台資料，不讀取使用者設定、密碼或真實站台清單。
- 瀏覽器請求全部攔截；只允許列出的靜態資產及模擬 API，其他請求使錄製失敗。沒有啟動 Client、Host、MCP 或遠端連線。
- 動畫中的在線狀態為示範資料，不代表實際網路或效能測試。流程停在密碼提示，不送出密碼或宣稱已收到遠端畫面。
- 下方步驟說明與藍色游標為錄製輔助標示，不屬於產品介面。

## 重新製作

需要 Node.js、可由 `require('playwright')` 載入的 Playwright、其 Chromium，以及有 Pillow 的 Python 3。於專案根目錄執行：

```sh
node scripts/render-readme-demo.cjs
```

也可使用已安裝的 Edge：`DEMO_BROWSER_CHANNEL=msedge node scripts/render-readme-demo.cjs`。Python 執行檔可透過 `PYTHON` 指定；腳本不會自動安裝套件或瀏覽器。

錄製幀與時間表存於 `.local-run/readme-demo-*`，成品為 `images/yourdesk-demo.gif`。錄製包含群組／搜尋結果、新增站台、主題、密碼視窗及非預期請求檢查；編碼驗證 GIF 可讀取、循環播放、尺寸、時長及小於 5 MiB。

重製已有 GIF 時會先建立 `.bak`，不覆蓋既有備份。完成後需查看動畫的主畫面、表單、暗色與密碼視窗，確認文字可讀且沒有遮擋；驗收成功才移除本次備份。只更新 GIF 與相關文件，不需要重新編譯 FFmpeg 或發布套件。原靜態圖保留於 `images/cap001.png`。
