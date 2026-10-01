# 合成音訊 Smoke 素材

這些檔案由 FFmpeg 產生 1 秒、440 Hz 的雙聲道合成訊號，不包含錄音或使用者內容。僅封裝到 androidTest APK，不放入正式 APK。

- `audio-opus.bin`：48 kHz、20 ms、96 kbps，使用 `libopus -application audio -frame_duration 20`；移除 Ogg 封裝及 OpusHead／OpusTags。
- `audio-aac.bin`：48 kHz、AAC-LC、128 kbps，使用 `aac -profile:a aac_low`；移除 ADTS 標頭。
- 共同輸入為 `sine=frequency=440:sample_rate=48000:duration=1`、`volume=0.35`、雙聲道。
- 檔案結構為重複的「4 bytes big-endian 封包長度＋單一壓縮音訊封包」，沒有 YDA1 標頭；協定 Host 會加入目前世代／序號。
- PCM 由 Smoke Host 即時產生，同樣使用 48 kHz／16-bit／雙聲道。
