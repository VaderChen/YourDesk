## 繁體中文

- 修正 Windows 連 Mac 命令列時，兩端系統時間差異可能使正常請求回報「invalid_request：指令格式無效」或逾時的問題。使用者已確認同步兩端時間後原問題消失。
- 桌面與 Android 的共用 P2P 指令改用能力協商的相對逾時預算，終端機、檔案及其他共用指令都適用；保留有限等待、重複請求保護及斷線取消，不自動重試有副作用的操作。
- **請更新控制端與被控端後重新連線。** 雙端均支援才啟用新流程；混用舊版仍須同步系統日期與時間。

套件包括 macOS Apple Silicon DMG、Windows x64／ARM64 Portable 與服務 ZIP、Linux x64／arm64 CLI Host ZIP、WinPE x64 實驗性 ZIP，以及 [Android arm64 ZIP](https://github.com/VaderChen/YourDesk/releases/download/1.26.1110-build-1451/YourDesk-1.26.1110-build-1451-android-arm64.zip)。macOS App 與 DMG 均以 Developer ID 簽章、通過 Apple 公證並附加公證票證；需允許螢幕錄製與輔助使用。Linux 套件沒有桌面 Viewer。

Windows 請完整解壓縮 `-portable.zip` 至可寫入的資料夾，保留 DLL 與授權文件，再執行 `YourDesk.exe`；需要 WebView2 Runtime。只提供 ZIP，沒有 NSIS 安裝 EXE；ZIP 內 EXE 仍未具可信程式碼簽章，可能出現系統警告。`-service.zip` 僅供已授權服務更新。線上 OTA 沿用服務交接；首次安裝或舊服務遷移需管理員授權，本地套件更新受保護服務時也需本次 UAC。首次授權系統操作後，執行期間服務可處理安全桌面，無須同時開啟登入前啟動；未授權的普通 Host 仍無此權限。本版不保證所有驅動上的 UAC 操作已通過實機驗收。舊版若需手動替換檔案，先使用「移除系統服務」再退出 APP；僅關閉登入前啟動或替換 ZIP，不會移除或更新既有服務副本。

Android 適用 Android 8.0 以上的 arm64 手機 Viewer。解壓縮 ZIP，再安裝其中正式簽章 APK；沿用相同簽章與 applicationId，可覆蓋更新而不需卸載。`1.26.1003 build 2028` 以前的版本須先手動更新一次；有更新器的版本可在首頁前景閒置時自動檢查及下載，連線或進入背景會停止。安裝仍須使用者及 Android 確認，必要時允許 YourDesk 安裝來源。手機被控、獨立檔案傳輸及 AV1 不在本版 Android 範圍。

驗證：桌面 P2P／terminal／Remote、Android core 測試通過；真實 DataChannel 模擬正負 24 小時時差、舊版相容、去重及 handler 取消通過；Mac 本機 P2P → PTY 的中文輸出、Ctrl+C、尺寸調整、大量輸出、關閉與重連通過。Windows x64／ARM64 指令模組編譯通過。新版尚未在原回報的 Windows／Mac 兩台實機驗收。手機剪貼簿僅限純文字（32 KiB）；遠端聲音、登入前服務與 WinPE 仍為實驗性。

[Validation / 驗證紀錄](https://github.com/VaderChen/YourDesk/blob/main/docs/RELEASE-VALIDATION-2026-11-10.md)

## English

- Fix valid Windows-to-Mac terminal requests being rejected as invalid_request or expired when device clocks differ. The reporter confirmed that synchronizing both clocks resolved the original failure.
- Desktop and Android shared P2P commands negotiate relative timeout budgets for terminals, files and other commands, retaining bounded waits, duplicate protection and disconnect cancellation without automatically retrying side effects.
- **Update both the controlling and remote devices, then reconnect.** Both ends must support this feature; connections involving older versions still require synchronized system clocks.

Packages: macOS Apple Silicon DMG; Windows x64/ARM64 Portable and service ZIPs; Linux x64/arm64 CLI Host ZIPs; experimental WinPE x64 ZIP; and [Android arm64 ZIP](https://github.com/VaderChen/YourDesk/releases/download/1.26.1110-build-1451/YourDesk-1.26.1110-build-1451-android-arm64.zip). Both the macOS app and DMG are Developer ID signed, Apple-notarized and stapled. Allow Screen Recording and Accessibility permissions. Linux packages do not include a desktop viewer.

On Windows, extract the entire `-portable.zip` into a writable folder, retain the DLLs and licenses, and run `YourDesk.exe`. WebView2 Runtime is required. Only ZIPs are published, without an NSIS installer; the EXEs still lack trusted code signing and may trigger system warnings. `-service.zip` is for authorized service updates only. Online OTA retains service handoff. Initial installation or legacy migration needs administrator approval; updating a protected service from a local package also needs UAC approval for that update. Once system control is authorized, a runtime service can handle the secure desktop without enabling pre-login startup; an unprivileged Host cannot. This does not establish successful UAC operation on every driver. Before manually replacing an older installation, remove the system service and quit the app. Disabling pre-login startup or replacing ZIP files alone does not remove or update an existing service copy.

Android is a phone Viewer for arm64 devices running Android 8.0 or later. Extract the ZIP and install the officially signed APK. The signing identity and applicationId are unchanged, allowing an in-place update without uninstalling. Versions before `1.26.1003 build 2028` need one manual upgrade. Versions with the updater check and download while the home screen is idle in the foreground; connecting or backgrounding stops this work. Installation still requires user and Android confirmation; allow YourDesk as an installation source if requested. Phone hosting, dedicated file transfer and AV1 are not included on Android.

Validation: desktop P2P/terminal/Remote and Android core tests passed, including real DataChannel requests with simulated ±24-hour clock skew, legacy compatibility, deduplication and handler cancellation. Local Mac P2P-to-PTY smoke covered Chinese output, Ctrl+C, resizing, bulk output, closing and reconnecting. Windows x64/ARM64 command modules compiled. The new build has not been verified on the original Windows/Mac device pair. Mobile clipboard supports plain text only (32 KiB). Remote audio, pre-login services and WinPE remain experimental.

[Validation / 驗證紀錄](https://github.com/VaderChen/YourDesk/blob/main/docs/RELEASE-VALIDATION-2026-11-10.md)

## 日本語

- Windows から Mac のターミナルへ接続する際、端末間の時刻差により正常な要求が invalid_request または期限切れとして拒否される問題を修正しました。報告者は両端の時刻同期で元の問題が解消したことを確認しました。
- デスクトップと Android の共通 P2P コマンドは相対タイムアウトを協商し、ターミナル・ファイル・その他のコマンドに適用します。有限の待機、重複実行防止、切断時のキャンセルを維持し、副作用のある操作を自動再試行しません。
- **操作側と接続先の両方を更新し、再接続してください。** 両端の対応が必要です。旧版との接続ではシステム日時の同期が必要です。

配布物：macOS Apple Silicon DMG、Windows x64／ARM64 Portable・サービス ZIP、Linux x64／arm64 CLI Host ZIP、実験的な WinPE x64 ZIP、[Android arm64 ZIP](https://github.com/VaderChen/YourDesk/releases/download/1.26.1110-build-1451/YourDesk-1.26.1110-build-1451-android-arm64.zip)。macOS APP と DMG は Developer ID 署名・Apple 公証済みで、公証チケットも添付済みです。画面収録とアクセシビリティの許可が必要です。Linux はデスクトップ Viewer を含みません。

Windows の `-portable.zip` を書き込み可能なフォルダーに全て展開し、DLL とライセンスを保持して `YourDesk.exe` を実行してください。WebView2 Runtime が必要です。NSIS インストーラーは提供せず、ZIP 内 EXE は信頼されたコード署名が未対応のため警告が出る場合があります。`-service.zip` は承認済みサービスの更新専用です。オンライン OTA はサービスの引き継ぎを維持します。初回導入・旧サービス移行には管理者の許可が必要で、ローカルパッケージから保護サービスを更新する際にも今回の UAC 承認が必要です。システム操作を許可すると実行中サービスが安全なデスクトップを扱え、ログイン前起動の有効化は必須ではありません。権限のない Host は対応できず、全ドライバーでの UAC 操作を実機確認済みとはしていません。旧版を手動置換する前はサービスを削除して APP を終了してください。ログイン前起動の無効化や ZIP 置換だけでは既存サービスの削除・更新は行われません。

Android 8.0 以降の arm64 スマートフォン向け Viewer です。ZIP を展開し、正式署名 APK をインストールしてください。同じ署名と applicationId を使用するため、アンインストールせず更新できます。`1.26.1003 build 2028` より前の版は一度手動更新が必要です。更新機能のある版はホーム画面が前面で待機中に検出・ダウンロードし、接続やバックグラウンド移行時に停止します。インストールにはユーザーと Android の確認が必要で、要求された場合は YourDesk からのインストールを許可してください。スマートフォンの被制御、専用ファイル転送、AV1 は Android 版の対象外です。

検証：デスクトップ P2P／terminal／Remote と Android core のテストを通過しました。実 DataChannel で正負 24 時間の時差、旧版互換、重複防止、handler キャンセルを確認しました。Mac ローカル P2P → PTY で中国語出力、Ctrl+C、サイズ変更、大量出力、終了・再接続を確認し、Windows x64／ARM64 コマンドモジュールをコンパイルしました。新版は報告元の Windows／Mac 2 台で未検証です。モバイルのクリップボードはテキストのみ（32 KiB）で、遠隔音声・ログイン前サービス・WinPE は実験的です。

[Validation / 驗證紀錄](https://github.com/VaderChen/YourDesk/blob/main/docs/RELEASE-VALIDATION-2026-11-10.md)

## 한국어

- Windows에서 Mac 터미널에 연결할 때 기기 시계 차이로 정상 요청이 invalid_request 또는 만료로 거부되는 문제를 수정했습니다. 제보자는 양쪽 시간을 동기화한 후 기존 문제가 해결되었음을 확인했습니다.
- 데스크톱과 Android 공통 P2P 명령은 상대 시간 제한을 협상하며 터미널, 파일 및 기타 명령에 적용합니다. 제한된 대기, 중복 실행 방지, 연결 종료 시 취소를 유지하고 부작용이 있는 작업을 자동 재시도하지 않습니다.
- **제어하는 기기와 원격 기기 모두 업데이트한 뒤 다시 연결하세요.** 양쪽 지원이 필요하며 구버전과 연결할 때는 시스템 날짜와 시간을 동기화해야 합니다.

배포물: macOS Apple Silicon DMG, Windows x64/ARM64 Portable·서비스 ZIP, Linux x64/arm64 CLI Host ZIP, 실험용 WinPE x64 ZIP, [Android arm64 ZIP](https://github.com/VaderChen/YourDesk/releases/download/1.26.1110-build-1451/YourDesk-1.26.1110-build-1451-android-arm64.zip). macOS 앱과 DMG 모두 Developer ID 서명, Apple 공증 및 공증 티켓 첨부를 완료했습니다. 화면 기록과 손쉬운 사용 권한이 필요합니다. Linux 패키지는 데스크톱 Viewer를 포함하지 않습니다.

Windows에서는 `-portable.zip` 전체를 쓰기 가능한 폴더에 풀고 DLL·라이선스를 유지한 채 `YourDesk.exe`를 실행하세요. WebView2 Runtime이 필요합니다. NSIS 설치 EXE 없이 ZIP만 제공하며, 내부 EXE에는 신뢰할 수 있는 코드 서명이 없어 시스템 경고가 표시될 수 있습니다. `-service.zip`은 승인된 서비스 업데이트 전용입니다. 온라인 OTA는 서비스 인계를 유지합니다. 최초 설치나 구형 서비스 이전에는 관리자 승인이 필요하고, 로컬 패키지로 보호된 서비스를 업데이트할 때도 해당 업데이트의 UAC 승인이 필요합니다. 시스템 제어를 승인하면 로그인 전 시작을 켜지 않아도 실행 중 서비스가 보안 데스크톱을 처리할 수 있습니다. 권한 없는 Host는 처리할 수 없으며, 모든 드라이버의 UAC 동작을 실기기 검증한 것은 아닙니다. 이전 설치를 수동 교체할 때는 시스템 서비스를 제거한 뒤 앱을 종료하세요. 로그인 전 시작을 끄거나 ZIP만 교체해도 기존 서비스 사본은 제거·업데이트되지 않습니다.

Android 8.0 이상 arm64 휴대폰용 Viewer입니다. ZIP을 풀고 정식 서명 APK를 설치하세요. 기존 서명과 applicationId를 유지하므로 앱 삭제 없이 업데이트할 수 있습니다. `1.26.1003 build 2028` 이전 버전은 한 번 수동 업데이트해야 합니다. 업데이트 기능이 있는 버전은 홈 화면이 전경에서 대기 중일 때 감지·다운로드하며 연결하거나 백그라운드로 이동하면 중단합니다. 설치는 사용자와 Android 확인이 필요하며, 요청 시 YourDesk를 설치 출처로 허용하세요. 휴대폰 원격 제어, 전용 파일 전송과 AV1은 Android 버전에 포함되지 않습니다.

검증: 데스크톱 P2P/terminal/Remote 및 Android core 테스트를 통과했습니다. 실제 DataChannel에서 ±24시간 시차, 구버전 호환, 중복 방지 및 handler 취소를 확인했습니다. Mac 로컬 P2P → PTY에서 중국어 출력, Ctrl+C, 크기 조정, 대량 출력, 종료 및 재연결을 검증하고 Windows x64/ARM64 명령 모듈을 컴파일했습니다. 새 빌드는 최초 제보 Windows/Mac 두 기기에서 아직 검증하지 않았습니다. 모바일 클립보드는 일반 텍스트(32 KiB)만 지원하며 원격 오디오, 로그인 전 서비스, WinPE는 실험 기능입니다.

[Validation / 驗證紀錄](https://github.com/VaderChen/YourDesk/blob/main/docs/RELEASE-VALIDATION-2026-11-10.md)
