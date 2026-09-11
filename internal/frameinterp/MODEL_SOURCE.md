# RIFE 4.25 Lite 來源

使用 Practical-RIFE 作者提供的 4.25.lite（2024-10-20）模型與隨附 IFNet_HDv3.py。

- 模型：https://drive.google.com/file/d/1zlKblGuKNatulJNFf5jdB-emp9AqGK05/view
- 作者：https://github.com/hzwer/Practical-RIFE
- 作者 README 說明模型下載適用同一 MIT 授權，全文附於 MODEL_LICENSE.txt 與套件的 ThirdPartyLicenses。
- warp 實作：https://github.com/hzwer/Practical-RIFE/blob/main/model/warplayer.py

轉換腳本 scripts/convert-rife.py：移除僅訓練使用的 teacher、caltime 權重，其餘 strict 載入。固定 512×384、t=0.5、FP16，輸出中間光流與 sigmoid 遮罩。接收端將畫面等比縮放及邊緣補齊，用模型預測後回到原始解析度做雙線性變形與遮罩融合；不把縮小的 RGB 輸出直接放大。此合成路徑為 YourDesk 修改，並非宣稱與作者原生全解析度結果一致。

原始檔與轉換模型雜湊分別存於 scripts/rife_model/SHA256SUMS 與 MODEL_SHA256SUMS。
