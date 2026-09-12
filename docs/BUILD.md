# 跨平台建置與打包

「更新三件套」代表更新文件、上傳 GitHub、發布 Release；完整步驟見 [發布流程](RELEASE-WORKFLOW.md)。

buildMac.command／buildWin.command／buildLinux.command 與 pack.command 統一版本格式為 `1.YY.MMDD build HHmm`（台北時間）。

對外架構名稱統一使用 `x64`（包含 Intel／AMD 64 位元平台），`arm64` 維持原名。建置目標接受 `x64` 與舊 `amd64` 名稱，發布目錄、檔名及 release.json 一律使用 `x64`；Go 的 `GOARCH=amd64` 與既有 `YOURDESK_*_AMD64` 工具鏈設定保留。更新程式優先辨識新 `x64` 套件，也相容舊 `amd64` 套件。舊版 APP 若尚未包含這項辨識修正，需手動下載新命名的安裝包。

## 使用方式

```sh
./buildMac.command       # 僅 Mac arm64，含簽章及公證
# 或
./buildWin.command       # Windows x64、WOA ARM64 與 WinPE x64 實驗版
./buildLinux.command     # Linux x64、arm64 命令列 Host（另擇一次建置）
./pack.command --no-build
# 或一次建置與打包
./pack.command
```

需要 Go 1.27.1 以上、Python 3.9 以上、zsh。macOS 桌面版使用 Xcode Command Line Tools。從 macOS 建置 Windows x64 桌面版需要 MinGW 的 gcc、g++、windres；Windows 封裝改用 NSIS 產生 Installer EXE，需要 makensis（macOS：brew install nsis）；可用 YOURDESK_MAKENSIS 指定編譯器。

macOS 僅支援 Apple Silicon（arm64），不支援 Intel（x64）；手動指定 darwin/x64 也會拒絕建置與封裝。

pack.command 預設目標：

| 目標 | 內容 | 封裝 |
| --- | --- | --- |
| macOS arm64 | 原生 Client UI、遠端顯示 | YourDesk.app、DMG |
| Windows x64 | 原生 Client UI、遠端顯示 | Installer EXE |
| Windows arm64（WOA） | 原生 ARM64 Client UI、遠端顯示 | ARM64 Installer EXE |
| WinPE x64（實驗性） | 原生 Win32 救援 Host | 便攜 ZIP |

Linux 提供 x64／arm64 命令列 Host ZIP；圖形處理與 REMOTE 尚未提供。Windows arm64（WOA）列入預設目標，需要 LLVM-MinGW 的 aarch64 編譯器及 windres。

一般桌面版的原生 Client／遠端顯示 強制使用 CGO；缺少工具鏈會明確停止，不產生無法開啟介面的替代執行檔。

```sh
YOURDESK_BUILD_TARGETS=darwin/arm64,windows/x64,windows/arm64 ./pack.command
YOURDESK_VERSION='1.26.0908 build 1800' ./pack.command
# 已建置版本重新封裝
YOURDESK_VERSION='1.26.0908 build 1800' ./pack.command --no-build
```

其他環境變數：

- `YOURDESK_CC_WINDOWS_ARM64`、`YOURDESK_CXX_WINDOWS_ARM64`：各目標的編譯器；後綴同樣支援 DARWIN_ARM64 等組合。
- `YOURDESK_CGO_CFLAGS_<目標>`、`YOURDESK_CGO_CXXFLAGS_<目標>`、`YOURDESK_CGO_LDFLAGS_<目標>`：目標專用編譯旗標。
- `YOURDESK_WINDOWS_ICON_PATH`：Windows ICO 圖示；預設為 assets/branding/yourdesk.ico，建置時嵌入每個 EXE。
- `YOURDESK_WINDRES_AMD64`、`YOURDESK_WINDRES_ARM64`：對應平台的資源編譯器。
- `YOURDESK_MAC_ICON_PATH`：自訂 icns；未指定時從專案圖像轉換。
- `YOURDESK_CODESIGN_IDENTITY`：macOS Developer ID Application 簽章身分。
- `YOURDESK_NOTARY_PROFILE`：必填，指定已儲存在本機 Keychain 的 notarytool profile；專案不保存個人設定名稱或認證資料。

未指定簽章身分時優先選擇 Keychain 的 Developer ID Application；找不到時停止建置，不退回 ad-hoc。固定 Developer ID 可讓 macOS 依簽章身分辨識更新版本；buildMac.command 的 App 與 pack.command 的 DMG 均必須完成 Apple 公證、附加並驗證票根與 Gatekeeper 評估；任一步驟失敗即停止。Windows 安裝程式及執行檔目前未做 Authenticode 簽章。

## 輸出與啟動

`dist/` 直接保存 `macos-arm64/`、`windows-x64/`、`windows-arm64/`、`winpe-x64/` 平台目錄，以及 release.json 與 SHA256SUMS，不再建立版本號子目錄。版本仍記錄於 release.json、應用程式及安裝包檔名。buildMac.command／buildWin.command／buildLinux.command 與預設 pack.command 會先清空專案的 dist，再於暫存目錄建置，成功後才發布產物。pack.command --no-build 依 dist/release.json 封裝現有產物，不清空 dist；若另指定 YOURDESK_VERSION，必須與現有版本一致。舊版版本子目錄需重新建置一次。清理會拒絕符號連結或非預期的 dist 路徑。對應本機平台的執行檔同步放入 bin。

macOS 開啟 YourDesk.app；Windows 開啟 YourDesk.exe（需 WebView2 Runtime）。入口會啟動同目錄的 yourdesk-client，Client 再管理 遠端顯示；關閉視窗仍常駐 Tray，從 Tray 選單結束程式。

Windows 套件包含繁體中文、英文、日文、韓文的 README.txt，使用 YourDesk.exe 啟動。

本專案僅提供 Client／遠端顯示，不含中央 Server 原始碼或執行檔。封裝使用明確檔案清單，不包含本機設定、連線密碼或 TLS 私鑰。Client 的 IP 直連 TLS 身分於本機首次使用時產生。雜湊清單只用於檔案完整性核對，不代表簽章或公證。

macOS 遠端顯示 使用內嵌 WebView 標題列。全螢幕以無邊框視窗模擬，保留同一個視窗與繪圖表面；雙擊標題區或使用縮放選單切換。工具列預設隱藏，頂端停留一秒浮現。原始鍵盤模式下 Ctrl／Cmd + Shift + F 與 F12 傳至遠端，不作為本機快捷鍵。完整操作與平台差異見[使用說明](../README.md)。

本機 runUITest.command、localRun.command、remoteRun.command、remoteClientOnly.command 皆在編譯後呼叫 scripts/sign-local.sh，使用相同 Developer ID 並驗證簽章後才執行。buildMac.command 產生的 macOS 原始執行檔與 App 也使用同一身分；直接手動 go build 不會自動執行簽章，執行前應呼叫此腳本。

Apple 平台目錄僅輸出 YourDesk.app（build）與 DMG（pack）；獨立執行檔與雜湊清單不放入 Apple 平台目錄，README.txt 隨附於平台目錄與 DMG。共用 release.json 與 SHA256SUMS 保留在 dist 根目錄；本機 bin 由 App 內的執行檔同步。

路徑以腳本所在的專案目錄為基準，移動專案後仍可建置。編譯使用 `-trimpath` 並移除除錯符號；公證設定由環境變數提供。DMG 的 `/Applications` 是 macOS 安裝捷徑，並非開發者本機目錄。

## 更新安裝包命名

自動更新讀取 `VaderChen/YourDesk` 最新正式 Release，不使用草稿或預發行版本。版本比較沿用 `1.YY.MMDD-build-HHmm` 或 `1.YY.MMDD build HHmm`；正式建置須注入正確版本。

「關於」的強制更新開關可讓手動檢查忽略版本比較，下載最新正式 Release。下載驗證完成後以紅字倒數 10 秒，使用者可取消或立即更新；到期後交給獨立更新程序，結束 APP、安裝並重新啟動。此流程會中斷遠端連線。

下載依目前系統與執行檔架構選擇資產，命名與 `scripts/release.py` 一致：

- `YourDesk-<版本>-macos-arm64.dmg`
- `YourDesk-<版本>-windows-x64-setup.exe`
- `YourDesk-<版本>-windows-arm64-setup.exe`

請將安裝包作為 Release 資產提供；原始碼壓縮包不作為安裝包。缺少符合架構的資產時，程式顯示原因並停用提醒視窗的下載按鈕。下載驗證資產大小及 API 提供的 SHA-256；簽章、公證與下載完整性驗證是不同步驟。

## 設定與驗證範圍

執行階段設定寫入作業系統使用者設定目錄的 `YourDesk/`，不寫入 App／EXE 目錄。主畫面、站台、遠端顯示 與更新狀態分開保存；打包時不得納入這些本機資料。

原始碼的編譯與 JavaScript／JSON 語法檢查不會自動完成套件簽章、公證或實機操作驗證。建置產物是否已封裝，請以實際執行的建置／打包流程為準。

兩個 build 入口固定各自平台，不受 YOURDESK_BUILD_TARGETS 覆寫。每次 build 都會清空 dist，先後執行兩者不會合併產物；需要同時建置並打包兩平台時直接執行 pack.command。Windows 建置入口預設包含 x64、ARM64 與 WinPE x64 實驗版；若只需要 ARM64，可使用 YOURDESK_BUILD_TARGETS=windows/arm64 搭配 pack.command，或直接呼叫 scripts/release.py build。

這三個 .command 入口為本機檔案，依 .gitignore 不上傳 GitHub。其他 checkout 可直接使用 `YOURDESK_BUILD_TARGETS=darwin/arm64 python3 scripts/release.py build` 或 `YOURDESK_BUILD_TARGETS=windows/x64,windows/arm64,winpe/x64 python3 scripts/release.py build`，打包使用 `python3 scripts/release.py pack`。

Windows 執行時透過 internal/childprocess 統一建立子程序，使用 CREATE_NO_WINDOW 禁止 PowerShell 等背景工具建立主控台；不設定 HideWindow，保留 Client／遠端顯示 的 GUI 顯示。裝置識別已改用 MachineGuid，必要時查詢 SMBIOS UUID，不再使用 CPU ID；詳見 [Windows 裝置 ID](WINDOWS-DEVICE-ID.md)。

## WinPE 實驗性便攜包

WinPE 不使用 Installer，固定封裝為 `dist/winpe-x64/YourDesk-<版本>-winpe-x64-experimental.zip`；內含救援 EXE、`start-yourdesk.cmd`、說明與授權文件。解壓後執行 CMD 初始化網路並開啟 Host。尚未完成 WinPE 實機驗證。

預設打包與 Windows 建置入口均包含此目標；只需 WinPE 時使用 `YOURDESK_BUILD_TARGETS=winpe/x64 python3 scripts/release.py pack`，此入口仍會清空 `dist`。WinPE 使用 Go 純交叉編譯，不需要 MinGW、NSIS 或 WebView2。

獨立入口 `./scripts/build-winpe.sh` 共用相同建置與 ZIP 邏輯，僅更新 `.local-run/winpe-dist/YourDesk-WinPE-x64-experimental.zip`，不清理 `dist`。功能及限制見 [WinPE 實作](WINPE-IMPLEMENTATION.md)。

## WOA 工具鏈

使用官方 LLVM-MinGW macOS universal 套件，解壓後可將 bin 加入 PATH，或設定 YOURDESK_LLVM_MINGW 為工具鏈根目錄。亦會搜尋 ~/.local/share/yourdesk/toolchains/llvm-mingw。本機已安裝 20260908 UCRT 版並核對官方 SHA-256。WOA 採原生 ARM64 編譯，不以 x64 模擬版替代；圖示資源也使用 ARM64 windres。建置產物為 dist/windows-arm64/，安裝包檔名使用 windows-arm64-setup.exe。Installer 本體使用 NSIS 引導程式，安裝的 YourDesk 三個 EXE 為原生 ARM64；仍須 WOA 實機確認硬體加速與周邊輸入相容性。

工具鏈來源：[LLVM-MinGW 官方發行頁](https://github.com/mstorsjo/llvm-mingw/releases)。本機已完成三個 ARM64 EXE 及 NSIS Installer 的編譯確認，尚未執行 WOA 實機功能測試。

## 畫面增強模型

macOS 建置會透過 go:embed 同時嵌入 QuickSRNet Small 與 SESR M5 Core ML 套件，不需另外下載模型。Windows 仍提供 FSR 1；Core ML 列為不支援。模型轉換與授權見 [QUICKSRNET.md](QUICKSRNET.md)。GitHub Release 說明提供繁中、英文、日文與韓文，專案技術文件維持繁體中文。

## 無終端機的本機 UI 試跑

雙擊專案根目錄的 `clientUI.app`，會在背景呼叫 `runUITest.command` 完成建置、簽章及啟動，不開啟 Terminal。輸出寫入 `.local-run/client-ui.log`，失敗時顯示提示。可用 `scripts/create-client-ui-launcher.sh` 重建此本機 App。直接雙擊 `runUITest.command` 仍依 macOS 行為開啟終端機，適合查看即時記錄。

## 補幀模型與本機啟動

RIFE 4.25 Lite 的轉換模型內嵌於 `internal/frameinterp`，來源、SHA256 與 MIT 授權一併保存。重建模型使用 `scripts/convert-rife.py` 與 `scripts/rife_model` 原始權重；一般 Go 建置不需要 Python 轉換環境。發行套件另附第三方模型授權。

`runUITest.command` 取代 `clientUI.command`；需要不顯示 Terminal 的本機入口時，以 `scripts/create-client-ui-launcher.sh` 產生 `clientUI.app`。本機記錄位於 `.local-run/client-ui.log`。

### 本機更新流程測試

從 `clientUI.app` 啟動時，會啟用 `YOURDESK_TEST_UPDATE=1`：只有手動按下「檢查更新」時放行版本比較。按「下載並自動更新」後，會下載正式套件、結束 APP、安裝並啟動正式版；這是真正的安裝，不是模擬。開發啟動器本身不被覆蓋。測試狀態不改寫正式更新紀錄。

若程式仍在執行，請先完整結束後再開啟 `clientUI.app`。一般安裝版與直接執行 `runUITest.command` 維持正常版本判斷；後者也可用 `YOURDESK_TEST_UPDATE=1 ./runUITest.command` 啟用相同測試。

Linux CLI 目標為 `linux/x64`、`linux/arm64`，兩者均包含於預設全平台建置，輸出 `YourDesk-版本-linux-架構-cli.zip`。套件只有 Host，不包含圖形 Viewer。各平台 README 提供繁中、英、日、韓使用方式；不附重複的使用說明.txt。
