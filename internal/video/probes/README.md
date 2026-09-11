# 編解碼能力探測影格

`h264.bin` 與 `hevc.bin` 為自行產生的 128×128 黑色單張獨立影格，已轉換成 YourDesk 的參數集與長度前綴 NAL 格式。只用於程式啟動時的硬體能力探測，不包含使用者畫面。

來源以 FFmpeg `color=c=black:s=128x128:r=1` 產生一張 `yuv420p` 影格；H.264 使用 libx264 baseline，HEVC 使用 libx265，兩者設定 `keyint=1:bframes=0`。封包保留 SPS／PPS（HEVC 另含 VPS）與 IDR，省略 SEI 編碼器描述。

固定影格讓解碼能力與本機是否具備編碼器分開驗證，避免只有硬體解碼能力的裝置被排除。
