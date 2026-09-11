# QuickSRNet Small 部署

畫面增強的 Core ML 後端同時嵌入 Qualcomm QuickSRNet Small 2× 與 SESR M5 2×，取代 PiperSR。選擇 Core ML 後顯示模型下拉選單，預設 QuickSRNet Small。設定仍使用 `coreml`，既有偏好設定相容；FSR 1 選項及三種增強策略維持不變。啟動時偵測 Apple Silicon／macOS 12 以上並載入模型；不可用或推論失敗時回退 FSR 1，遠端顯示 提示顯示實際後端。

## 模型與大小

- 官方 QuickSRNet Small 2× FP32 checkpoint：282,333 bytes（約 276 KiB）。
- 比較用 SESR M5 2× 原始 checkpoint：4,028,066 bytes（約 3.84 MiB）。
- QuickSRNet Core ML FP16 套件：55,992 bytes（約 54.7 KiB），為檔案內容合計，不含檔案系統配置空間與系統編譯快取。
- SESR M5 Core ML FP16 套件：51,783 bytes（約 50.6 KiB）。
- 原始 checkpoint 含訓練狀態；不能直接拿它的大小當作部署權重大小。

SESR M5 同樣轉為 Core ML FP16；兩者各自於啟動時探測可用性。模型偏好 `coreMLModel` 使用 `quicksrnet-small` 或 `sesr-m5`，空值相容舊設定並採用 QuickSRNet Small。選擇其他超解析度時隱藏模型選單但保留偏好；模型切換會清除舊影格結果。

## 輸入與執行

RGB 輸入正規化為 0～1，模型結果轉回 0～255 RGB；固定 512 × 512 輸入、1024 × 1024 輸出。既有原生執行器採 16 像素重疊邊界、裁除邊界後拼接，再縮放到 遠端顯示 顯示尺寸。串流寬高 75% 與策略參數不因更換模型而改變。

Core ML 自動選擇運算裝置，不保證使用 ANE。先前比較報告的 MPS 時間不是此 Core ML 管線的測量值。本次模型替換只需更新 遠端顯示 端；遠端原有串流參數能力要求維持不變。

## 適用情境與實際使用觀察

依目前實際使用回饋，遠端解析度越高，畫面增強的效益越明顯，例如 1440p、4K 桌面。寬、高各縮為 75% 後，像素總數剩下原本的 56.25%，減少 43.75%。比例相同，但解析度越高，省下的絕對像素數越多：1080p 每幀減少 907,200 個像素，4K 每幀減少 3,628,800 個像素。

實際 FPS 與網路流量取決於所選策略、編碼器、硬體及網路瓶頸；遠端顯示 的放大處理成本也會隨輸入尺寸增加。低解析度桌面或密集的小字，縮圖後可能出現較明顯的細節損失。

## 重建與授權

分別執行 `python3 scripts/convert-quicksrnet.py quick` 與 `python3 scripts/convert-quicksrnet.py sesr`，使用 scripts/sr_models 內保存的官方架構與 checkpoint；需要 PyTorch、coremltools。轉換完成後更新 `internal/superres/MODEL_SHA256SUMS`。模型來源、修改說明與 BSD-3-Clause 授權位於 `internal/superres/MODEL_LICENSE.txt`，發行套件的 ThirdPartyLicenses 同時提供授權全文。

本次已完成 Core ML 編譯及 Go／JavaScript 語法建置檢查，未另做執行效能測試。
