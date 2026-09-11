## 新增 MCP / New MCP support / MCP 新機能 / MCP 새 기능

**繁體中文**：新增 MCP：AI Agent 可連到遠端、取得畫面、操作鍵盤滑鼠、斷線、診斷及調整設定。設定 → MCP 設定中啟用，預設關閉；白名單預設開啟且僅允許 127.0.0.1，每個請求須使用獨立 Token。畫面預設隱藏，Tray 提供操作提示與開啟／隱藏選項。

**English**：New MCP support lets AI agents connect remotely, capture screenshots, control keyboard/mouse, disconnect, run diagnostics and manage settings. Enable it in MCP Settings; it defaults to off. The allowlist defaults to on with only 127.0.0.1, and every request requires a separate token. Windows are hidden by default, with tray indicators and show/hide controls.

**日本語**：新しい MCP 機能により、AI Agent からリモート接続、画面取得、キーボード・マウス操作、切断、診断、設定変更ができます。MCP 設定で有効にしてください。初期状態は無効、IP 許可リストは有効で 127.0.0.1 のみ許可し、専用トークンが必要です。画面は初期状態で非表示となり、トレイで通知と表示切替ができます。

**한국어**：새 MCP 기능으로 AI Agent가 원격 연결, 화면 캡처, 키보드·마우스 제어, 연결 해제, 진단 및 설정 변경을 할 수 있습니다. MCP 설정에서 활성화하세요. 기본값은 꺼짐이며 IP 허용 목록은 켜짐으로 127.0.0.1만 허용하고 별도 토큰이 필요합니다. 화면은 기본적으로 숨겨지며 트레이에서 알림과 표시 전환을 제공합니다.

本機入口 / Local endpoint: `http://127.0.0.1:12345/mcp`
[MCP 設定、驗證與工具說明 / Setup and tools](https://github.com/VaderChen/YourDesk/blob/main/docs/MCP.md)

## 剪貼簿改善 / Clipboard improvements / クリップボード改善 / 클립보드 개선

**剪貼簿同步改善：Mac 對 Mac 的文字、圖片及雙向檔案複製已由使用者實機確認可用。**

**Clipboard synchronization improved: the user confirmed that Mac-to-Mac text, image and bidirectional file copy work on real devices.**

**クリップボード同期を改善しました。Mac 間のテキスト、画像、双方向ファイルコピーが実機で利用できることをユーザーが確認しました。**

**클립보드 동기화 개선: 사용자가 실제 기기에서 Mac 간 텍스트, 이미지 및 양방향 파일 복사가 가능함을 확인했습니다.**

## 其他更新 / Other changes

- macOS 與 Windows 自動更新：下載完成後以紅字倒數 10 秒，可取消或立即更新；接著結束程式、安裝及重新啟動。Automatic updates now include a red 10-second countdown with Cancel and Update now controls.
- 「關於」新增強制更新開關，手動檢查時可忽略版本比較，下載最新正式 Release。Added a Force update switch for manually downloading the latest release regardless of the installed version.
- MCP 接管畫面可依設定顯示或隱藏，Tray 圖示與提示反映 Agent 操作狀態。MCP sessions support configurable visibility and tray controls.
- 修正 MCP 滑鼠按鍵編號與底層輸入映射。Corrected MCP mouse-button mapping.
- 延續既有 FPS 擷取加速、Double Buffer 串流與實驗性 2× 補幀功能；本版未新增效能基準數據。Existing capture FPS optimizations, Double Buffer streaming and experimental 2× interpolation remain available; no new performance benchmark is claimed.

## 已知問題 / Known issue / 既知の問題 / 알려진 문제

**繁體中文**：部分電腦仍可能出現重複 Host 占用，導致無法被連線。暫時請重新啟動該電腦後再試，不保證恢復；正在處理。

**English**: Duplicate Host occupancy may still prevent incoming connections on some computers. Restart the affected computer and retry as a temporary workaround; recovery is not guaranteed. We are working on this issue.

**日本語**：一部のパソコンでは Host の重複占有で接続できない場合があります。暫定的に対象のパソコンを再起動して再試行してください。復旧は保証されません。対応中です。

**한국어**: 일부 컴퓨터에서는 중복 Host 점유로 원격 연결을 받을 수 없습니다. 임시로 해당 컴퓨터를 재부팅한 뒤 다시 시도하세요. 복구를 보장하지 않으며 해결 중입니다.

## 套件與驗證 / Packages and validation

macOS Apple Silicon DMG、Windows x64／ARM64 Installer。macOS App 與 DMG 已簽章並通過 Apple 公證。三平台編譯與 JavaScript／JSON／Python 語法檢查通過。Mac 雙向檔案複製可用由使用者實機確認；此輪未重新執行 Windows 實機更新測試。

Includes a signed and notarized macOS Apple Silicon DMG and Windows x64/ARM64 installers. Platform builds and syntax checks passed. The user confirmed Mac-to-Mac bidirectional file copy on real devices; Windows update installation was not retested on hardware in this release pass.
