# macOS 遠端檔案剪貼簿實作調查

調查日期：2026-09-10。查閱上游 master 原始碼；上游後續可能變動。本次為研究與設計整理，未修改執行程式。

發布前更新：使用者再次實測並確認 Mac 對 Mac 文字、圖片及雙向檔案複製已可用。以下保留當時的研究背景與候選方案，不代表目前仍失效，也不代表已採用 RustDesk 的供檔架構。

## 現有證據與尚未確認事項

YourDesk 已能取得遠端檔案清單，亦曾透過 WebDAV 路徑完整讀取測試檔案並核對 SHA-256。然而使用者從遠端 Finder 複製後，近端 Finder 仍無法正常貼上。這表示傳輸成功不足以證明剪貼簿整合成功。

先前本機 loopback smoke test 沒有涵蓋兩台 Mac 的正常 Finder 複製與貼上，因此不能用它宣稱問題解決。剪貼簿發布 API 回傳成功，也不能代表 Finder 後續仍能取得正確項目。

目前沒有足夠證據將原因歸給 AnyDesk。也尚未證明是主執行緒、WebDAV 或系統剪貼簿同步中的任何單一原因。

## RustDesk：macOS 專用的延後傳輸流程

查閱來源：

- [macOS 設計說明](https://github.com/rustdesk/rustdesk/blob/master/libs/clipboard/src/platform/unix/macos/README.md)
- [剪貼簿 context](https://github.com/rustdesk/rustdesk/blob/master/libs/clipboard/src/platform/unix/macos/pasteboard_context.rs)
- [資料提供者](https://github.com/rustdesk/rustdesk/blob/master/libs/clipboard/src/platform/unix/macos/item_data_provider.rs)
- [貼上觀察器](https://github.com/rustdesk/rustdesk/blob/master/libs/clipboard/src/platform/unix/macos/paste_observer.rs)
- [傳輸任務](https://github.com/rustdesk/rustdesk/blob/master/libs/clipboard/src/platform/unix/macos/paste_task.rs)

實際 macOS 實作使用 NSPasteboardItemDataProvider。當系統索取 file URL 時，提供者建立暫存佔位檔，並把檔案 URL 交給剪貼簿。FSEvents 觀察佔位檔在目的地被建立，再觸發檔案清單與內容傳輸。傳輸任務另外處理分段請求、暫存下載檔、取消與進度。

這與 YourDesk「先掛載本機 WebDAV，再發布掛載內的 URL，讓讀檔觸發傳輸」不同。RustDesk 的通用 clipboard README 提到 FUSE，但 macOS 專用 README 與原始碼採用上述方式，不能混為一談。

限制：目前觀察器以使用者家目錄為監看根目錄，且設有 30 秒觀察控制逾時。這使外接磁碟等家目錄外目的地，以及延後貼上的行為，需要另行驗證；不能假設照搬後就全面相容。

## Maccy：原生檔案剪貼簿發布

來源：[Clipboard.swift](https://github.com/p0deje/Maccy/blob/master/Maccy/Clipboard.swift)。Maccy 是剪貼簿管理器，不負責遠端傳輸，適合比較 macOS 發布方式。

其 copy 方法標示 @MainActor；多檔案使用每檔一個 NSPasteboardItem，設定 fileURL 後以 writeObjects 一次發布，並加入自己的來源標記。程式也刻意分開處理檔案 URL 與其他格式，避免格式化文字重複貼上。

可借鏡之處是集中發布、明確的項目結構與來源識別。這些是設計參考，不是 YourDesk 根因證明；先前僅把 NSURL 改成 NSPasteboardItem 的嘗試並未解決實機問題。

## 分階段處理建議

1. 先建立可觀測的發布流程：同一操作串起收到清單、準備內容、發布剪貼簿、發布後讀回、Finder 取用及傳輸完成。成功訊息須區分各階段。剪貼簿已被使用者新複製內容取代時，不能以重試覆蓋它。
2. 以實體本機暫存檔作為受控診斷基準：在相同接收流程發布已下載完成的測試檔，與 WebDAV URL 比較。用途是隔離掛載因素，不代表正式產品要預先下載所有大檔案。
3. 集中檢查 macOS 剪貼簿發布的執行環境、生命週期與來源識別。每次只驗證一個假設，避免多種改動混在一起後無法判斷原因。
4. 若證據指向掛載方案，才評估獨立的延後供檔後端。RustDesk 的佔位檔方案可參考，但需先涵蓋家目錄外目的地、重複貼上、同名檔、取消及斷線清理，不能直接取代現有流程。

驗收以兩台 Mac 的實際 Finder 操作為準：單檔、多檔、資料夾、連續複製、延後貼上，並核對目的檔內容。原生視窗與 MCP 隱藏模式均須涵蓋。這是後續驗證計畫，本次未重新執行這些測試。

研究結論：當時的資料不足以定位根因，因此沒有更換供檔架構。使用者後續已確認雙向檔案複製可用；若問題再現，可依上述階段重新定位，避免直接套用未驗證的架構變更。
