# AV1 編解碼

AV1 編碼與解碼能力分開判斷，不以晶片名稱或硬解成功推定硬編可用。

| 平台 | 硬體編碼 | 軟體編碼 | 硬體解碼 | 軟體解碼 |
| --- | --- | --- | --- | --- |
| Windows x64／ARM64 | Media Foundation，依實測能力 | FFmpeg／libaom | Media Foundation，失敗後退回軟解 | FFmpeg／libaom |
| macOS Apple Silicon | VideoToolbox，實際建立與編碼測試 | FFmpeg／libaom | FFmpeg 的 VideoToolbox AV1 後端，實測成功才算可用 | FFmpeg／libaom |

macOS 硬解使用 VideoToolbox，沒有另以 CPU 解碼冒充硬體加速；FFmpeg 負責 AV1 格式解析與工作階段。硬解不可用時改用 libaom，GOP 中途切換若缺少參考影格，先要求關鍵影格恢復。硬體編碼不支援時顯示不支援，仍可使用 AV1 軟編。

「傳輸到遠端的編碼」提供 AV1 硬體／AV1 軟體。自動模式沿用雙端硬體優先，其次編碼端硬體，再考慮解碼端硬體與軟體候選。硬編、軟編的失敗紀錄各自獨立，且只套用相同尺寸。手動選擇不由傳輸策略改選其他影片編碼器；最後保留 JPEG 相容回退。

軟編採用即時模式、無前瞻等待，保留 FPS、碼率與 GOP 設定。CPU 編碼是否能達到使用者指定 FPS 取決於解析度與設備效能，功能測試通過不代表即時效能達標。

編解碼分析的 AV1 軟體編碼、軟體解碼與硬體路徑分開測試。解碼使用固定壓縮樣本，不依賴當次編碼成功。頁面隱藏 128×128，只列較接近實際用途的 1920×1080 項目；內部保留 128×128 快速能力探測與協商。實際串流尺寸仍獨立判斷，不從小尺寸成功外推 4K 支援。

## 建置與發行

正式 Windows／macOS 桌面建置均使用 `turbojpeg,ffmpeg` 標籤。FFmpeg 維持 LGPL 組態，不啟用 GPL 或 nonfree；libaom 同時啟用 AV1 編碼與解碼。設定變更使用新的快取識別，已有完整快取不重編函式庫。

Windows 隨附三個 FFmpeg DLL；macOS 隨附三個 dylib，放在 App/Contents/Frameworks，執行檔使用相對 rpath 載入，函式庫先簽章再簽 App。來源、授權及重建腳本一併封裝；macOS 使用者替換函式庫後須對修改後的 App 重新簽章。原有 H.264／HEVC 的 macOS 路徑保持使用 VideoToolbox。

## 本輪驗證範圍

- macOS AV1 軟編／軟解：128×128、1920×1080，多影格 GOP、顏色與尺寸核對。
- macOS 正式 AV1 接收路徑：簽章後的程式實測 1080p 使用 VideoToolbox 硬體解碼；軟編、軟解亦成功。本機硬編建立回傳 -12908，保守標示不支援。
- 編碼尺寸變更、偶數補邊、關閉後拒絕編碼，與硬編失敗仍保留軟編候選。
- macOS 相對路徑函式庫載入 Smoke 與正式 Client／Remote 編譯。
- Windows x64／ARM64 正式 Client／Remote 跨編譯、影片測試編譯、DLL 相依驗證。

Windows 的 AV1 軟編執行與 GPU 驅動行為仍需 Windows 實機驗收。上述修改納入 1.26.0913 build 1512 發行套件。
