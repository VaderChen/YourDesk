# 編解碼能力探測影格

`h264.bin` 與 `hevc.bin` 為自行產生的 128×128 黑色單張獨立影格，已轉換成 YourDesk 的參數集與長度前綴 NAL 格式。只用於程式啟動時的硬體能力探測，不包含使用者畫面。

來源以 FFmpeg `color=c=black:s=128x128:r=1` 產生一張 `yuv420p` 影格；H.264 使用 libx264 baseline，HEVC 使用 libx265，兩者設定 `keyint=1:bframes=0`。封包保留 SPS／PPS（HEVC 另含 VPS）與 IDR，省略 SEI 編碼器描述。

固定影格讓解碼能力與本機是否具備編碼器分開驗證，避免只有硬體解碼能力的裝置被排除。

`h264-1080.annexb`／`hevc-1080.annexb` 使用相同黑色來源與 keyint=1、bframes=0 參數，尺寸為 1920×1080，保留 Annex B 格式供 Windows 獨立軟解測試；128×128 沿用既有 fixture。這些是建置時固定資料，使用者執行測試不需要 FFmpeg。

AV1 的 `av1-128.obu`、`av1-1080.obu` 為 FFmpeg/libsvtav1 產生的黑色單影格 low-overhead OBU（`color=black:s=尺寸:r=30 -frames:v 1 -c:v libsvtav1 -preset 12 -f obu`），用於獨立軟解測試。

`h264-1080.bin`、`hevc-1080.bin` 由同尺寸 Annex B 樣本轉成 YourDesk wire 格式，供正式接收端獨立解碼；`jpeg-128.jpg`、`jpeg-1920.jpg` 為 Go 標準 JPEG、品質 70 的固定黑色影格。執行解碼測試時不呼叫任何編碼器。
