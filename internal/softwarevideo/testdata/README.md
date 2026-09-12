# CPU 解碼 GOP Smoke 影格

三份 `.frames` 皆為 128×128、30 fps、三張 `testsrc2` 合成影格。每張前面放四位元組 big-endian 壓縮長度，後面是 Annex B（H.264／HEVC）或 low-overhead OBU（AV1）。第一張為 key frame，後兩張保留參考關係。

產生方式：FFmpeg `-f lavfi -i testsrc2=s=128x128:r=30 -frames:v 3 -g 30 -bf 0`，H.264 使用 `libx264 -tune zerolatency`，HEVC 使用 `libx265 -x265-params bframes=0`，AV1 使用 `libsvtav1 -preset 12 -svtav1-params pred-struct=1`。H.264／HEVC 依 ffprobe 的 packet pos/size 分割；AV1 由 IVF 的逐影格長度分割。

以上 encoder 僅用於產生測試資料，不連結或包含於產品；Windows 發行庫只包含 LGPL FFmpeg 解碼器和 BSD libaom。
