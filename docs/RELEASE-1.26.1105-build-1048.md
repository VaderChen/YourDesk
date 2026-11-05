## 繁體中文

- 改善 macOS 的組合按鍵擷取與遠端修飾鍵狀態，減少按鍵被本機處理、遠端只收到單鍵的情況。
- 修正 Windows 方向／編輯／功能鍵的掃描碼與接收處理；改善 Windows／macOS 的 Ctrl、Command 映射，虛擬鍵盤明確選取的按鍵不再被再次映射。
- Android 同步支援原始掃描碼與 Ctrl／Alt／Shift／Meta 狀態，補上部分鍵盤缺少實體 code 的對應及失焦重設。保留中文輸入法與舊 Host 相容性。請更新兩端；Windows 服務副本也應透過 APP 更新。

套件包括 macOS Apple Silicon DMG、Windows x64／ARM64 Portable 與服務 ZIP、Linux x64／arm64 CLI Host ZIP、WinPE x64 實驗性 ZIP，以及 [Android arm64 ZIP](https://github.com/VaderChen/YourDesk/releases/download/1.26.1105-build-1048/YourDesk-1.26.1105-build-1048-android-arm64.zip)。macOS App 與 DMG 均以 Developer ID 簽章、通過 Apple 公證並附加公證票證；需允許螢幕錄製與輔助使用。Linux 套件沒有桌面 Viewer。

Windows 請完整解壓縮 `-portable.zip` 至可寫入的資料夾，保留 DLL 與授權文件，再執行 `YourDesk.exe`；需要 WebView2 Runtime。只提供 ZIP，沒有 NSIS 安裝 EXE；ZIP 內 EXE 仍未具可信程式碼簽章，可能出現系統警告。`-service.zip` 僅供已授權服務更新。線上 OTA 沿用服務交接；首次安裝或舊服務遷移需管理員授權，本地套件更新受保護服務時也需本次 UAC。首次授權系統操作後，執行期間服務可處理安全桌面，無須同時開啟登入前啟動；未授權的普通 Host 仍無此權限。本版不保證所有驅動上的 UAC 操作已通過實機驗收。舊版若需手動替換檔案，先使用「移除系統服務」再退出 APP；僅關閉登入前啟動或替換 ZIP，不會移除或更新既有服務副本。

Android 適用 Android 8.0 以上的 arm64 手機 Viewer。解壓縮 ZIP，再安裝其中正式簽章 APK；沿用相同簽章與 applicationId，可覆蓋更新而不需卸載。`1.26.1003 build 2028` 以前的版本須先手動更新一次；有更新器的版本可在首頁前景閒置時自動檢查及下載，連線或進入背景會停止。安裝仍須使用者及 Android 確認，必要時允許 YourDesk 安裝來源。手機被控、剪貼簿同步、獨立檔案傳輸及 AV1 不在本版 Android 範圍。

驗證涵蓋 Android 31 項瀏覽器 Smoke、Go 能力公告與桌面按鍵測試、各平台正式建置及套件檢查。macOS App／DMG 簽章與公證、Android 簽章延續性／版本碼／16 KB 對齊、全部附件 SHA-256 均經核對。

瀏覽器橋接為替身，尚未完成本版 Android 外接鍵盤到 Windows／macOS 的完整實機驗收。系統在事件到達 APP 前攔截的快捷鍵不保證能轉送，Windows Ctrl+Alt+Del 仍需既有 SAS 授權流程。不同鍵盤配置、IME、Windows／WOA 驅動與 UAC／SCM、Linux／WinPE 實機、Android 16／16 KB 裝置及數小時負載仍待驗收。遠端聲音、登入前服務與 WinPE 維持實驗性。

[驗證紀錄](https://github.com/VaderChen/YourDesk/blob/main/docs/RELEASE-VALIDATION-2026-11-05.md)。

## English

- Improve macOS chord capture and remote modifier state, reducing shortcuts handled locally or received as plain keys remotely.
- Fix Windows navigation, editing and function-key scan codes and injection. Improve Windows/macOS Ctrl and Command mapping; explicit virtual-keyboard selections are no longer mapped again.
- Android now sends raw scan codes and Ctrl/Alt/Shift/Meta snapshots, supports unambiguous keys when physical code is missing, and resets keys on focus loss. IME text and legacy Host compatibility remain. Update both ends and any Windows service through the app updater.

Packages: macOS Apple Silicon DMG; Windows x64/ARM64 Portable and service ZIPs; Linux x64/arm64 CLI Host ZIPs; experimental WinPE x64 ZIP; and [Android arm64 ZIP](https://github.com/VaderChen/YourDesk/releases/download/1.26.1105-build-1048/YourDesk-1.26.1105-build-1048-android-arm64.zip). Both the macOS app and DMG are Developer ID signed, Apple-notarized and stapled. Allow Screen Recording and Accessibility permissions. Linux packages do not include a desktop viewer.

On Windows, extract the entire `-portable.zip` into a writable folder, retain the DLLs and licenses, and run `YourDesk.exe`. WebView2 Runtime is required. Only ZIPs are published, without an NSIS installer; the EXEs still lack trusted code signing and may trigger system warnings. `-service.zip` is for authorized service updates only. Online OTA retains service handoff. Initial installation or legacy migration needs administrator approval; updating a protected service from a local package also needs UAC approval for that update. Once system control is authorized, a runtime service can handle the secure desktop without enabling pre-login startup; an unprivileged Host cannot. This does not establish successful UAC operation on every driver. Before manually replacing an older installation, remove the system service and quit the app. Disabling pre-login startup or replacing ZIP files alone does not remove or update an existing service copy.

Android is a phone Viewer for arm64 devices running Android 8.0 or later. Extract the ZIP and install the officially signed APK. The signing identity and applicationId are unchanged, allowing an in-place update without uninstalling. Versions before `1.26.1003 build 2028` need one manual upgrade. Versions with the updater check and download while the home screen is idle in the foreground; connecting or backgrounding stops this work. Installation still requires user and Android confirmation; allow YourDesk as an installation source if requested. Phone hosting, clipboard sync, dedicated file transfer and AV1 are not included on Android.

Validation covers 31 Android browser smoke cases, Go capability and desktop-key tests, release builds and package checks. macOS app/DMG signing and notarization, Android signing continuity/version code/16 KB alignment, and all asset SHA-256 values are verified.

Browser tests use a mocked bridge; this release has not completed end-to-end device acceptance with an Android external keyboard controlling Windows/macOS. Keys intercepted by the OS before reaching the app are not guaranteed to be forwarded. Windows Ctrl+Alt+Del still requires the existing authorized SAS flow. Keyboard layouts, IMEs, Windows/WOA drivers and UAC/SCM, physical Linux/WinPE systems, Android 16/16 KB devices and multi-hour workloads still need validation. Remote audio, pre-login services and WinPE remain experimental.

[Validation record](https://github.com/VaderChen/YourDesk/blob/main/docs/RELEASE-VALIDATION-2026-11-05.md)。

## 日本語

- macOS の組み合わせキー取得と遠隔側の修飾キー状態を改善し、ローカル処理や単独キーとして届く問題を軽減しました。
- Windows の方向・編集・機能キーのスキャンコードと注入処理を修正しました。Windows／macOS の Ctrl・Command 対応を改善し、仮想キーボードで明示したキーの二重変換を防ぎます。
- Android もスキャンコードと Ctrl／Alt／Shift／Meta 状態を送信し、物理 code がない場合の明確なキー対応とフォーカス喪失時の解除を追加しました。IME と旧 Host との互換性は維持します。両端を更新し、Windows サービスも APP 更新で更新してください。

配布物：macOS Apple Silicon DMG、Windows x64／ARM64 Portable・サービス ZIP、Linux x64／arm64 CLI Host ZIP、実験的な WinPE x64 ZIP、[Android arm64 ZIP](https://github.com/VaderChen/YourDesk/releases/download/1.26.1105-build-1048/YourDesk-1.26.1105-build-1048-android-arm64.zip)。macOS APP と DMG は Developer ID 署名・Apple 公証済みで、公証チケットも添付済みです。画面収録とアクセシビリティの許可が必要です。Linux はデスクトップ Viewer を含みません。

Windows の `-portable.zip` を書き込み可能なフォルダーに全て展開し、DLL とライセンスを保持して `YourDesk.exe` を実行してください。WebView2 Runtime が必要です。NSIS インストーラーは提供せず、ZIP 内 EXE は信頼されたコード署名が未対応のため警告が出る場合があります。`-service.zip` は承認済みサービスの更新専用です。オンライン OTA はサービスの引き継ぎを維持します。初回導入・旧サービス移行には管理者の許可が必要で、ローカルパッケージから保護サービスを更新する際にも今回の UAC 承認が必要です。システム操作を許可すると実行中サービスが安全なデスクトップを扱え、ログイン前起動の有効化は必須ではありません。権限のない Host は対応できず、全ドライバーでの UAC 操作を実機確認済みとはしていません。旧版を手動置換する前はサービスを削除して APP を終了してください。ログイン前起動の無効化や ZIP 置換だけでは既存サービスの削除・更新は行われません。

Android 8.0 以降の arm64 スマートフォン向け Viewer です。ZIP を展開し、正式署名 APK をインストールしてください。同じ署名と applicationId を使用するため、アンインストールせず更新できます。`1.26.1003 build 2028` より前の版は一度手動更新が必要です。更新機能のある版はホーム画面が前面で待機中に検出・ダウンロードし、接続やバックグラウンド移行時に停止します。インストールにはユーザーと Android の確認が必要で、要求された場合は YourDesk からのインストールを許可してください。スマートフォンの被制御、クリップボード同期、専用ファイル転送、AV1 は Android 版の対象外です。

Android のブラウザー Smoke 31 項目、Go の能力公告・デスクトップキー試験、正式ビルドと配布検査を実施しました。macOS APP／DMG の署名・公証、Android の署名継続性・バージョンコード・16 KB 配置、全添付ファイルの SHA-256 を確認しています。

ブラウザー試験のブリッジは代替実装であり、本版の Android 外付けキーボードから Windows／macOS までの実機受け入れ試験は未完了です。APP に届く前に OS が処理するキーの転送は保証しません。Windows Ctrl+Alt+Del は既存の SAS 許可手順が必要です。各キー配列、IME、Windows／WOA ドライバーと UAC／SCM、Linux／WinPE 実機、Android 16／16 KB 端末、数時間の負荷は引き続き確認が必要です。リモート音声、ログイン前サービス、WinPE は実験的機能です。

[検証記録](https://github.com/VaderChen/YourDesk/blob/main/docs/RELEASE-VALIDATION-2026-11-05.md)。

## 한국어

- macOS의 조합 키 캡처와 원격 수정 키 상태를 개선하여 로컬에서 처리되거나 원격에 단일 키만 전달되는 문제를 줄였습니다.
- Windows 방향·편집·기능 키의 스캔 코드와 입력 처리를 수정했습니다. Windows/macOS의 Ctrl·Command 매핑을 개선하고 가상 키보드에서 명시적으로 선택한 키의 중복 매핑을 방지합니다.
- Android도 스캔 코드와 Ctrl/Alt/Shift/Meta 상태를 전송하며, 물리 code가 없는 명확한 키의 대응 및 포커스 상실 시 초기화를 추가했습니다. IME와 구형 Host 호환성을 유지합니다. 양쪽 앱과 Windows 서비스를 앱 업데이트로 갱신하세요.

배포물: macOS Apple Silicon DMG, Windows x64/ARM64 Portable·서비스 ZIP, Linux x64/arm64 CLI Host ZIP, 실험용 WinPE x64 ZIP, [Android arm64 ZIP](https://github.com/VaderChen/YourDesk/releases/download/1.26.1105-build-1048/YourDesk-1.26.1105-build-1048-android-arm64.zip). macOS 앱과 DMG 모두 Developer ID 서명, Apple 공증 및 공증 티켓 첨부를 완료했습니다. 화면 기록과 손쉬운 사용 권한이 필요합니다. Linux 패키지는 데스크톱 Viewer를 포함하지 않습니다.

Windows에서는 `-portable.zip` 전체를 쓰기 가능한 폴더에 풀고 DLL·라이선스를 유지한 채 `YourDesk.exe`를 실행하세요. WebView2 Runtime이 필요합니다. NSIS 설치 EXE 없이 ZIP만 제공하며, 내부 EXE에는 신뢰할 수 있는 코드 서명이 없어 시스템 경고가 표시될 수 있습니다. `-service.zip`은 승인된 서비스 업데이트 전용입니다. 온라인 OTA는 서비스 인계를 유지합니다. 최초 설치나 구형 서비스 이전에는 관리자 승인이 필요하고, 로컬 패키지로 보호된 서비스를 업데이트할 때도 해당 업데이트의 UAC 승인이 필요합니다. 시스템 제어를 승인하면 로그인 전 시작을 켜지 않아도 실행 중 서비스가 보안 데스크톱을 처리할 수 있습니다. 권한 없는 Host는 처리할 수 없으며, 모든 드라이버의 UAC 동작을 실기기 검증한 것은 아닙니다. 이전 설치를 수동 교체할 때는 시스템 서비스를 제거한 뒤 앱을 종료하세요. 로그인 전 시작을 끄거나 ZIP만 교체해도 기존 서비스 사본은 제거·업데이트되지 않습니다.

Android 8.0 이상 arm64 휴대폰용 Viewer입니다. ZIP을 풀고 정식 서명 APK를 설치하세요. 기존 서명과 applicationId를 유지하므로 앱 삭제 없이 업데이트할 수 있습니다. `1.26.1003 build 2028` 이전 버전은 한 번 수동 업데이트해야 합니다. 업데이트 기능이 있는 버전은 홈 화면이 전경에서 대기 중일 때 감지·다운로드하며 연결하거나 백그라운드로 이동하면 중단합니다. 설치는 사용자와 Android 확인이 필요하며, 요청 시 YourDesk를 설치 출처로 허용하세요. 휴대폰 원격 제어, 클립보드 동기화, 전용 파일 전송과 AV1은 Android 버전에 포함되지 않습니다.

Android 브라우저 Smoke 31개, Go 기능 공지·데스크톱 키 테스트, 정식 빌드와 패키지 검사를 수행했습니다. macOS 앱/DMG 서명·공증, Android 서명 연속성·버전 코드·16 KB 정렬 및 모든 첨부 파일의 SHA-256을 확인했습니다.

브라우저 테스트는 대체 브리지를 사용하며, 이번 버전의 Android 외장 키보드에서 Windows/macOS까지의 실기기 인수 검증은 완료하지 않았습니다. 앱에 도착하기 전에 OS가 가로채는 키의 전달은 보장하지 않습니다. Windows Ctrl+Alt+Del은 기존 SAS 권한 절차가 필요합니다. 키보드 배열, IME, Windows/WOA 드라이버와 UAC/SCM, Linux/WinPE 실기기, Android 16/16 KB 기기 및 수시간 부하는 추가 검증이 필요합니다. 원격 오디오, 로그인 전 서비스와 WinPE는 실험 기능입니다.

[검증 기록](https://github.com/VaderChen/YourDesk/blob/main/docs/RELEASE-VALIDATION-2026-11-05.md)。
