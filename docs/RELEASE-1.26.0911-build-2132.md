本版合併 build 2104 的所有功能，並修正被遠端連線後停止圖示造成的工具列跑版。

- **緊急修正**：設定與停止按鈕共用同一欄，顯示／隱藏停止圖示不再擠壓快速連線與新增站台按鈕。

- 站台提供桌面／命令列圖示，雙擊可選擇模式；命令列使用 xterm.js 獨立視窗，可同時開啟不同站台。
- 新增 Linux x64、arm64 CLI Host ZIP；目前接受命令列連線，不包含 Linux 圖形處理或 REMOTE。請以一般使用者執行，root 不提供互動 Shell。
- CLI 支援 `-secret "密碼"` 指定本次密碼；依 LC_ALL、LC_MESSAGES、LANG 選擇繁中、英文、日文、韓文，其他語系使用英文。
- MCP 支援命令列連線、互動終端機、站台能力查詢，並提供桌面／命令列模式選擇建議。
- 新增「斷線後自動關閉視窗」，預設關閉；保留桌面畫面與終端機輸出供查看。
- 可用模式以圖示顏色提示；斷線後等待 Host 恢復時顯示轉圈圈。統一欄位外觀。
- 套件 README 提供四語使用方式，不再附重複的使用說明.txt。

## 套件

- macOS Apple Silicon：已簽章、公證的 DMG。
- Windows x64／ARM64：安裝程式 EXE。
- WinPE x64：實驗性 ZIP。
- Linux x64／arm64：CLI Host ZIP。

## English

Hotfix: showing the incoming-connection Stop icon no longer shifts the toolbar. Includes all features from build 2104.

Independent terminal windows, Linux x64/arm64 CLI Host ZIPs, and MCP interactive terminal/capability tools are now available. Choose Desktop or Terminal from a site; older unsupported peers show an update prompt. CLI accepts `-secret "password"` and detects four supported locales. Disconnected windows remain open by default; recovering connections show a spinner. Package READMEs contain usage in four languages. Linux graphics and REMOTE are not included; run as a regular user.

## 日本語

緊急修正：接続中の停止アイコン表示によるツールバーの崩れを修正しました。build 2104 の全機能を含みます。

独立ターミナル、Linux x64／arm64 CLI Host ZIP、MCP 対話ターミナルと能力照会を追加しました。接続先でデスクトップ／ターミナルを選べます。CLI は `-secret "パスワード"` と 4 言語の自動選択に対応します。切断後の画面保持は初期設定で有効で、復帰待ちは回転アイコンで表示します。README は 4 言語の使い方を記載します。Linux の GUI と REMOTE は含みません。一般ユーザーで実行してください。

## 한국어

긴급 수정: 수신 연결 중지 아이콘 표시 시 도구 모음이 밀리는 문제를 수정했습니다. build 2104의 모든 기능을 포함합니다.

독립 터미널 창, Linux x64/arm64 CLI Host ZIP 및 MCP 대화형 터미널과 기능 조회를 추가했습니다. 데스크톱/터미널 모드를 선택할 수 있습니다. CLI는 `-secret "암호"`와 4개 언어 자동 선택을 지원합니다. 연결 해제 후 창은 기본적으로 유지되며 복구 대기는 회전 아이콘으로 표시합니다. README는 4개 언어의 사용법을 제공합니다. Linux GUI와 REMOTE는 포함되지 않으며 일반 사용자로 실행해야 합니다.
