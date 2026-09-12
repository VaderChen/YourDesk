# YourDesk 1.26.0912 build 1720

## 繁體中文

### 功能更新

- 新增進階設定，集中顯示使用提示、按鍵映射、畫面滿版及斷線後關閉視窗等選項。
- 「登入前啟動」整併原本的未登入開機功能，直接管理系統服務，不再要求先開啟登入後自動啟動，並明確標示「實驗性功能」。
- Windows 新增開機自動啟動服務，使用者未登入時即可啟動 Host；包含桌面代理、登入／鎖定桌面切換、異常重啟及子程序回收。啟用、停用需管理員授權，更新或解除安裝前需先停用。
- CLI 保留使用者層級的登入後啟動設定；Linux 使用 systemd 使用者服務，與登入前系統服務分開管理。
- 同步納入上一批介面改善：記住站台群組、改善對話框焦點、本機連線狀態、MCP 位址排列，以及關於頁的 GitHub 與支持開發連結。

### 錯誤修正與多國語言

- 修正「支持開發」連結點擊後未開啟瀏覽器；GitHub 與支持開發連結共用系統預設瀏覽器入口。

- 修正 macOS 簽章驗證參數，避免有效的已簽署 APP 被錯誤拒絕；加強登入工作階段及既有螢幕錄製／輔助使用權限檢查。
- 修正服務啟用成功後仍把一般功能說明顯示為警告的問題；底層 `exit status` 不再直接顯示於該操作提示。
- 本機開發啟動器與執行檔沿用固定 Developer ID；啟動器加入公證流程。
- 安裝包、發布目錄及更新辨識統一使用 `x64`，並相容舊 `amd64` 套件名稱。尚未包含此修正的舊版 APP，請手動下載新安裝包。
- 相關介面與使用說明同步繁中、英文、日文及韓文；各平台套件附四語授權全文，使用條件請參閱隨附 LICENSE。

### 套件與驗證範圍

提供 macOS Apple Silicon DMG、Windows x64／ARM64 安裝程式、WinPE x64 實驗性 ZIP，以及 Linux x64／arm64 CLI ZIP。已確認編譯、套件雜湊與 macOS 簽章／公證票根。

登入前服務仍為實驗性功能。使用者已確認 Windows 服務成功啟動，初步實機測試可用；長時間穩定性與完整情境仍待觀察。Windows／macOS 的開機、登入／登出、鎖定畫面擷取及輸入仍須完整驗證。桌面切換可能需要重新連線；不支援 FileVault／BitLocker 開機前解鎖。Windows 服務僅管理實體主控台、不提供 SYSTEM 遠端終端機或合成 Ctrl+Alt+Del；Linux 仍為 CLI Host。

## English

Also fixed the support-development link not opening a browser; GitHub and support links now share the system-browser entry point. The user reports that the Windows pre-login service starts and initial on-device checks work; long-term stability and complete scenario coverage remain under observation.

### Features

- Added Advanced Settings for usage hints, key mapping, window fitting and closing windows after disconnect.
- Start before login now directly manages the system service, consolidates the previous pre-login option and no longer requires a separate login-startup setting. It is labeled experimental.
- Added an automatic Windows service that starts the Host at boot before sign-in, with desktop agents, login/lock-screen transitions, restart recovery and child-process cleanup. Administrator approval is required; disable it before updating or uninstalling.
- Retained independent user-level login startup for CLI, including Linux systemd user services.
- Includes remembered site groups, improved dialog focus, local status labels, MCP address alignment and GitHub/support links in About.

### Fixes and localization

- Fixed macOS code-signature validation incorrectly rejecting signed apps; improved session and existing Screen Recording/Accessibility permission checks.
- Successful service changes no longer display informational text as a warning. Raw exit-status details are kept out of this operation's user-facing message.
- Local development binaries use a stable Developer ID; the launcher supports notarization.
- Release files and directories use x64. The updated updater recognizes both x64 and legacy amd64 names; older apps without this fix require a manual download.
- Updated Traditional Chinese, English, Japanese and Korean UI/documentation. Packages include the four-language license texts; see the bundled LICENSE for usage terms.

Packages: macOS Apple Silicon DMG, Windows x64/ARM64 installers, experimental WinPE x64 ZIP, and Linux x64/arm64 CLI ZIPs. Builds, package hashes and macOS signing/notarization tickets were checked. Pre-login access remains experimental and requires full on-device boot, sign-in/out and lock-screen validation. Desktop transitions may require reconnection. FileVault/BitLocker pre-boot unlock is unsupported. Windows manages the physical console only, does not expose a SYSTEM terminal or synthesize Ctrl+Alt+Del. Linux remains a CLI Host.

## 日本語

開発支援リンクでブラウザーが開かない問題も修正し、GitHub と共通のシステムブラウザー起動処理にしました。Windows ログイン前サービスの起動と初期実機確認は利用者から動作報告があり、長時間の安定性と全シナリオは引き続き確認が必要です。

### 機能更新

- 詳細設定に操作ヒント、キー割り当て、画面調整、切断後のウィンドウ終了をまとめました。
- ログイン前の起動をシステムサービスの直接管理に統合しました。ログイン後の自動起動設定は不要で、実験的機能として表示します。
- Windows に起動時の自動サービスを追加しました。ログイン前の Host 起動、デスクトップ代理、ログイン／ロック画面の切り替え、異常時の再起動、子プロセスの回収に対応します。管理者の許可が必要で、更新やアンインストール前に無効化してください。
- CLI のユーザー単位のログイン後起動と Linux systemd ユーザーサービスを維持します。グループ記憶、ダイアログのフォーカス、状態表示、MCP アドレス配置、概要の GitHub／支援リンクも含みます。

### 修正・多言語対応

- 署名済み macOS APP を拒否する検証引数を修正し、セッションと画面収録／アクセシビリティ許可の確認を改善しました。
- サービス変更成功時の説明を警告として表示する問題を修正しました。終了ステータスなどの内部情報は操作メッセージに表示しません。
- ローカル開発用の固定 Developer ID と起動器の公証フローを整備しました。
- 配布名を x64 に統一し、更新処理は旧 amd64 名にも対応します。未対応の旧 APP は手動でダウンロードしてください。繁体字中国語・英語・日本語・韓国語の表示、説明、同梱ライセンスを更新しました。

macOS DMG、Windows x64／ARM64 Installer、実験的 WinPE x64 ZIP、Linux x64／arm64 CLI ZIP を提供します。ビルド・ハッシュ・macOS 署名／公証票を確認済みです。ログイン前接続は実験的で、起動・ログイン／ログアウト・ロック画面の実機検証は未完了です。切り替え時は再接続が必要になる場合があります。FileVault／BitLocker の起動前解除、Windows の複数 RDP セッション、SYSTEM ターミナル、Ctrl+Alt+Del 合成には非対応です。Linux は CLI Host のみです。利用条件は同梱 LICENSE を確認してください。

## 한국어

개발 후원 링크가 브라우저를 열지 않던 문제를 수정하고 GitHub 링크와 시스템 브라우저 실행 처리를 통합했습니다. 사용자가 Windows 로그인 전 서비스 시작과 초기 실제 장치 동작을 확인했으며, 장시간 안정성과 전체 시나리오는 계속 관찰이 필요합니다.

### 기능 업데이트

- 고급 설정에 사용 도움말, 키 매핑, 화면 맞춤 및 연결 해제 후 창 닫기를 모았습니다.
- 로그인 전 시작을 시스템 서비스 관리로 통합했습니다. 별도의 로그인 후 자동 시작 설정이 필요하지 않으며 실험적 기능으로 표시합니다.
- Windows 부팅 시 자동 시작 서비스를 추가했습니다. 로그인 전 Host 시작, 데스크톱 에이전트, 로그인/잠금 화면 전환, 비정상 종료 후 재시작 및 자식 프로세스 정리를 포함합니다. 관리자 승인이 필요하며 업데이트나 제거 전에 꺼야 합니다.
- CLI 사용자별 로그인 후 시작과 Linux systemd 사용자 서비스를 유지합니다. 그룹 기억, 대화상자 포커스, 상태 표시, MCP 주소 정렬 및 정보 페이지의 GitHub/후원 링크 개선도 포함합니다.

### 수정 및 다국어

- 서명된 macOS APP을 거부하던 검증 인수를 수정하고 세션 및 화면 기록/손쉬운 사용 권한 확인을 개선했습니다.
- 서비스 변경 성공 후 안내를 경고로 표시하던 문제를 수정했습니다. 종료 상태 등 내부 정보는 해당 작업 메시지에 표시하지 않습니다.
- 로컬 개발용 고정 Developer ID와 실행기 공증 절차를 정비했습니다.
- 배포 이름을 x64로 통일하고 업데이트 기능은 이전 amd64 이름도 지원합니다. 이 수정이 없는 구버전은 수동 다운로드가 필요합니다. 번체 중국어, 영어, 일본어, 한국어 UI/문서와 동봉 라이선스를 제공합니다.

macOS DMG, Windows x64/ARM64 설치 프로그램, 실험적 WinPE x64 ZIP 및 Linux x64/arm64 CLI ZIP을 제공합니다. 빌드, 해시 및 macOS 서명/공증 티켓을 확인했습니다. 로그인 전 연결은 실험적이며 부팅, 로그인/로그아웃 및 잠금 화면의 실제 장치 검증은 미완료입니다. 화면 전환 시 재연결이 필요할 수 있습니다. FileVault/BitLocker 부팅 전 잠금 해제, Windows 다중 RDP 세션, SYSTEM 터미널 및 Ctrl+Alt+Del 합성은 지원하지 않습니다. Linux는 CLI Host만 제공합니다. 사용 조건은 동봉 LICENSE를 확인하세요.
