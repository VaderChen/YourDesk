# 深度檢查修復與驗證紀錄

本紀錄描述 2026-09-20 修正前次檢查確認的 10 項問題。當次修正保留既有修改且未同步 GitHub；2026-09-21 依使用者要求，將本紀錄與累積修正納入原始碼同步，不代表已建立新 Release。

## 修正對照

| 問題 | 修正 | 主要回歸驗證 |
| --- | --- | --- |
| JPEG patch 因背壓遺失後永久缺塊 | Viewer 檢查序號與合成基底；單一恢復工作者節流重試；舊 keyframe 不可清除新丟幀；Host 每 5 秒補靜止畫面 | 真實 DeltaEncoder、兩槽 Consumer 與像素合成，最後 patch 遺失且桌面不再變動仍恢復 |
| Control 開啟競態與訊息超車 | 狀態檢查、入列、flush、送出使用同一把序列化鎖；滿載回錯，不淘汰已接受事件 | 開啟／排隊競態、順序、失敗重試、100 輪並行開啟 |
| H.264 微小 NAL 放大記憶體 | 尺寸解析改逐個 NAL 走訪；H.264／HEVC wire 上限 4,096 個 NAL，轉換前拒絕超限 | SPS 後繼續驗證、NAL 邊界、尺寸解析 0 B/op、0 allocs/op |
| DXGI 無新幀仍配置整張 RGBA | 改成 take → copy → release，取得新幀才配置；成功影格保持獨立所有權 | 無幀零配置、成功影格不互相覆寫、失敗時 release |
| Windows 解碼矩陣與 range 不一致 | 依 media type 映射 BT.601／709、full／limited；不支援矩陣交既有 FFmpeg 備援；編碼轉換及 metadata 同步 BT.709 | 彩色色塊、缺少 metadata、錯誤屬性、不支援矩陣、原生 ASan／UBSan |
| MCP 子程序 stdin 阻塞整個管理介面 | 每子程序一個 writer、16 格佇列、64 KiB 訊息上限、10 秒截止；等待移到全域鎖外 | 真實匿名管線滿載、取消、短寫、worker／pending 回收 |
| remote_action 繞過 terminal ownership | 只接受已列出的桌面 action；terminal／shell／files 不可借此進入私有命令路徑 | MCP SDK 邊界與 terminal instance／owner 測試 |
| Windows 回復漏掉 DLL 與新版殘留 | 管理清單對齊 NSIS，備份／回復 DLL、授權、notes、marker、Uninstall 等；保留未知使用者檔案 | NSIS 清單雙向一致性；新增隔離 PowerShell 回復 fixture |
| Portable ZIP 誤進自動安裝 | 下載驗證後開啟解壓目錄，不倒數、不關閉 APP／連線；同步介面文案 | 無倒數、無 quit、下載驗證與開啟狀態 |
| macOS 宣告最低版本與產物不符 | 配合 go.mod 的 Go 1.27.1，主程式、FFmpeg／libaom、TurboJPEG、Info.plist 統一最低 13.0；建置／封裝前檢查 Mach-O | 真正重建、最低版本／架構／平台檢查、拒絕環境旗標覆寫 |

Go 1.27 的最低版本為 macOS 13；不能只把 C 編譯參數改成 12 就宣稱支援 macOS 12。依據：[Go 1.27 官方說明](https://go.dev/doc/go1.27#darwin)。

額外修正：macOS 簽章採明確的程式／動態庫清單，避免 exFAT 的 `._*.dylib` 被當作 Mach-O 簽署；有回歸測試。

## 已完成驗證

所有命令均由專案目錄執行，路徑不依賴固定磁碟位置。

```sh
python3 -m unittest discover -s scripts -p 'test_*.py'
./runUITest.command --build-only
python3 scripts/ffmpeg.py darwin/arm64 go test -race -vet=all -count=1 -timeout=180s ./...
python3 scripts/ffmpeg.py darwin/arm64 go test -race -count=20 -timeout=180s ./cmd/remote ./internal/p2p ./internal/hostsession ./internal/clientui ./internal/desktop ./internal/video -run 'Test(JPEG|FrameRecovery|ControlSender|ProcessInput|AgentPipe|RemoteActionRejects|PendingCapture|H264|WireRejects|Idle)'
```

- Python 28 項通過；完整含 TurboJPEG／FFmpeg 的 Go race／vet 通過；整合後指定回歸重複 20 輪通過。
- 恢復通知每輪 100,000 次、control 每輪 100 次並行開啟，均在重複高壓測試中通過。
- 真實本機 Pion：剪貼簿滿載＋ping、慢速控制工作者＋ping 各 10 輪通過。
- MCP 管線指定測試額外 30 輪 race 通過；跨組獨立複核另跑 15 輪通過。
- Windows x64／ARM64 的 clientui 測試執行檔已用 `CGO_ENABLED=0` 交叉編譯成功；這不等於 Windows 原生影音編譯通過。
- 三個 macOS 程式及三個 FFmpeg 動態庫的 Mach-O 最低版本均已核對；主程式簽章通過。2026-09-20 驗證當時未執行公證、DMG 發布或 GitHub 同步；後續原始碼同步不改變公證／發行狀態。
- JS／JSON 與差異格式檢查通過。各組成功後移除本輪 `.deepfix.bak`，既有其他備份不動。

## 行為與驗證限制

- 靜止桌面最後封包遺失的週期恢復需 **Host 也更新**；只更新 Viewer 無法知道網路遺失的最後一個 patch。
- 取消尚未寫入的控制命令只會跳過該筆；取消正在寫入的命令則關閉該子程序 stdin，避免留下半筆 JSON。其他 session 不受影響。
- 傳輸恢復工作者不阻塞影像 DataChannel 回呼；但底層同步 Send 不支援 context，極端卡住時仍依賴 peer.Close 解除，未宣稱任意底層故障都能由 2 秒 context 強制中斷。
- 本機無 Windows 原生工具鏈／GPU／PowerShell；未完成 DXGI／Media Foundation 的 Windows SDK 編譯、GPU 實測或安裝器實際回復。PowerShell fixture 明確 skip；已新增 Windows GPU 硬／軟解彩色色塊測試供實機執行。
- 本機使用 macOS 27；實際 macOS 13 啟動、完整 GUI 遠端連線仍待對應機器驗證。Mach-O 最低版本核對不替代舊系統實測。
- 建置仍有既有第三方 webview 語法、重複 objc 連結，以及新版 VideoToolbox 型別的 availability 警告；新版 VT class 為 weak import，實際啟用有可用性檢查。

## FFmpeg 產物保留

最低版本屬於真正建置設定變更，因此本輪產生新的 FFmpeg 快取；舊二進位保留，不以清快取方式強迫每次重建。新產物完成後，後續建置／測試顯示重用，並以雜湊及修改時間比對確認未改寫。FFmpeg／libaom 工作目錄在成功後清除，保留已安裝二進位、標頭、metadata、來源封存與重建資料。

## 2026-09-21 同步前重驗

桌面與 Android 累積修正一併同步前，再次執行主機端回歸，未重建 FFmpeg、AAR 或 APK，也未進行簽章／公證／Release 發布。

```sh
go test -mod=readonly -race -vet=all -count=1 -timeout=180s -skip '^TestBackgroundDetectorSmoke$' ./...
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s scripts -p 'test_*.py'
```

- 桌面 Go 全套 race／vet 通過；本次跳過三輪背景硬體探測，原本需明確環境變數啟用的真 Host／GUI smoke 仍依原設計跳過。這不是重新跑一次完整硬體驗收。
- Python scripts 共 38 個測試通過，包含新增的 10 個 Android 產物檢查測試。
- Android core／anet 23 個 Go 測試、16 個 Edge 案例、Java policy／signaling 與 10,608 項 decoder 契約斷言通過；範圍與裝置限制見 [Android 修正紀錄](ANDROID-REVIEW-FIXES-2026-09-20.md)。
- 只同步來源、測試及文件；本機 `internal.zip`、快取、備份、AppleDouble `._` 副檔及私密設定不納入提交。
