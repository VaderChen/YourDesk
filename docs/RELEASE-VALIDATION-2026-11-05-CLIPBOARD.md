# 剪貼簿與遠端注音驗證

版本：[1.26.1105 build 2314](RELEASE-1.26.1105-build-2314.md)。

- 桌面：macOS Viewer 不停用全部系統快捷鍵、不搶走本機文字欄位焦點；針對遠端 Control＋方向、功能或空白鍵擷取。Windows Host 的修飾鍵回到實體掃描碼；Mac Host 依來源快照更新修飾旗標。Windows 控 Mac 的 Ctrl+Space 保留 Control，不轉成 Spotlight 的 Cmd+Space。
- 桌面貼上：依遠端平台與鍵盤映射辨識複製、剪下、貼上。遠端剛複製而近端內容沒變時，立即貼上沿用遠端內容；近端新複製的資料仍先等遠端確認。Win+V 歷程與 Ctrl+Alt+V 不觸發標準貼上屏障。
- Android：手機螢幕鍵盤保留已完成文字；外接鍵盤在支援原始鍵的 Host 傳送實體字母、數字、標點、選字空白，讓遠端組字。桌面前景依能力協商使用有序控制通道同步純文字（32 KiB）；讀取手機剪貼簿限明確的貼上快捷鍵。

桌面 rawkey、input、clipboard、remote 套件測試及剪貼簿順序 race 測試通過。隔離的 macOS 具名剪貼簿與實際 P2P 通道分別傳送「遠端複製：注音」與「近端複製：注音」，兩個方向均收到寫入確認；未接觸一般系統剪貼簿。Android core、原生橋接及 33 項 Chromium Smoke 通過，包含外接鍵盤注音鍵位、選字空白、完成文字、貼上順序與失焦釋放。

證據位於本機 `.local-run/clipboard-ime-fix/` 與 `.local-run/release-1.26.1105-build-2314/`。未將本機雙向傳輸、替身瀏覽器測試或跨平台編譯視為跨機 Windows／macOS 實際注音候選選取驗收；仍需更新兩端後測試現場的輸入來源與鍵盤配置。Android 圖片、檔案剪貼簿及系統保留快捷鍵不在手機文字流程範圍。
