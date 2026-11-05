## 繁體中文

- 桌面修正複製／剪下後立即貼上可能用舊近端內容覆蓋遠端，以及 Win+V 等快捷鍵誤觸貼上傳輸。macOS Viewer 僅在遠端畫面有焦點時擷取指定 Control 組合鍵，本機編輯欄位保有焦點。
- 修正 Windows 修飾鍵掃描碼與 Mac 的 Ctrl+Space／修飾旗標處理，讓遠端輸入法收到實體按鍵。Android 外接鍵盤字母、標點與選字空白鍵交由遠端輸入法組字，螢幕鍵盤仍傳送完成的中文。
- Android 前景桌面支援雙向純文字剪貼簿（32 KiB）。遠端複製的文字可在手機貼上；手機文字在按遠端貼上鍵時先傳送，不在背景讀取。請更新控制端與被控端。

套件包括 macOS Apple Silicon DMG、Windows x64／ARM64 Portable 與服務 ZIP、Linux x64／arm64 CLI Host ZIP、WinPE x64 實驗性 ZIP，以及 [Android arm64 ZIP](https://github.com/VaderChen/YourDesk/releases/download/1.26.1105-build-2314/YourDesk-1.26.1105-build-2314-android-arm64.zip)。macOS App 與 DMG 均以 Developer ID 簽章、通過 Apple 公證並附加公證票證；需允許螢幕錄製與輔助使用。Linux 套件沒有桌面 Viewer。

Windows 請完整解壓縮 `-portable.zip` 至可寫入的資料夾，保留 DLL 與授權文件，再執行 `YourDesk.exe`；需要 WebView2 Runtime。只提供 ZIP，沒有 NSIS 安裝 EXE；ZIP 內 EXE 仍未具可信程式碼簽章，可能出現系統警告。`-service.zip` 僅供已授權服務更新。線上 OTA 沿用服務交接；首次安裝或舊服務遷移需管理員授權，本地套件更新受保護服務時也需本次 UAC。首次授權系統操作後，執行期間服務可處理安全桌面，無須同時開啟登入前啟動；未授權的普通 Host 仍無此權限。本版不保證所有驅動上的 UAC 操作已通過實機驗收。舊版若需手動替換檔案，先使用「移除系統服務」再退出 APP；僅關閉登入前啟動或替換 ZIP，不會移除或更新既有服務副本。

Android 適用 Android 8.0 以上的 arm64 手機 Viewer。解壓縮 ZIP，再安裝其中正式簽章 APK；沿用相同簽章與 applicationId，可覆蓋更新而不需卸載。`1.26.1003 build 2028` 以前的版本須先手動更新一次；有更新器的版本可在首頁前景閒置時自動檢查及下載，連線或進入背景會停止。安裝仍須使用者及 Android 確認，必要時允許 YourDesk 安裝來源。手機被控、獨立檔案傳輸及 AV1 不在本版 Android 範圍。

驗證：本機真實 P2P 通道雙向傳送不同中文文字，透過隔離 macOS 剪貼簿確認寫入；桌面按鍵／剪貼簿測試、Android core 與 33 項瀏覽器 Smoke 通過。未完成兩台實機的 Windows／macOS 注音切換、候選字及 Android 外接鍵盤端到端驗收。手機剪貼簿僅支援純文字，不支援圖片與檔案；遠端聲音、登入前服務與 WinPE 仍為實驗性。

[驗證紀錄](https://github.com/VaderChen/YourDesk/blob/main/docs/RELEASE-VALIDATION-2026-11-05-CLIPBOARD.md)

## English

- Fix desktop copy/cut followed by a quick paste overwriting remote content with stale local data. Win+V history no longer triggers a paste transfer. The macOS Viewer limits system capture to selected Control chords while the remote canvas has focus; local editors keep focus.
- Restore Windows modifier scan codes and correct Mac Ctrl+Space and modifier flags for remote input methods. Physical Android letters, punctuation and candidate-selection space now reach the remote IME; on-screen keyboards still send committed text.
- Android foreground desktop sessions support bidirectional plain-text clipboard (32 KiB). Remote copied text can be pasted on the phone; phone text is sent when invoking remote paste, without background reads. Update both ends.

Packages: macOS Apple Silicon DMG; Windows x64/ARM64 Portable and service ZIPs; Linux x64/arm64 CLI Host ZIPs; experimental WinPE x64 ZIP; and [Android arm64 ZIP](https://github.com/VaderChen/YourDesk/releases/download/1.26.1105-build-2314/YourDesk-1.26.1105-build-2314-android-arm64.zip). Both the macOS app and DMG are Developer ID signed, Apple-notarized and stapled. Allow Screen Recording and Accessibility permissions. Linux packages do not include a desktop viewer.

On Windows, extract the entire `-portable.zip` into a writable folder, retain the DLLs and licenses, and run `YourDesk.exe`. WebView2 Runtime is required. Only ZIPs are published, without an NSIS installer; the EXEs still lack trusted code signing and may trigger system warnings. `-service.zip` is for authorized service updates only. Online OTA retains service handoff. Initial installation or legacy migration needs administrator approval; updating a protected service from a local package also needs UAC approval for that update. Once system control is authorized, a runtime service can handle the secure desktop without enabling pre-login startup; an unprivileged Host cannot. This does not establish successful UAC operation on every driver. Before manually replacing an older installation, remove the system service and quit the app. Disabling pre-login startup or replacing ZIP files alone does not remove or update an existing service copy.

Android is a phone Viewer for arm64 devices running Android 8.0 or later. Extract the ZIP and install the officially signed APK. The signing identity and applicationId are unchanged, allowing an in-place update without uninstalling. Versions before `1.26.1003 build 2028` need one manual upgrade. Versions with the updater check and download while the home screen is idle in the foreground; connecting or backgrounding stops this work. Installation still requires user and Android confirmation; allow YourDesk as an installation source if requested. Phone hosting, dedicated file transfer and AV1 are not included on Android.

Validation: a real local P2P connection transferred different Chinese texts in both directions and confirmed writes to an isolated macOS pasteboard. Desktop keyboard/clipboard tests, Android core tests and 33 browser smoke cases passed. Cross-device Windows/macOS Zhuyin switching, candidate selection and end-to-end Android external keyboard use have not been verified on hardware. Mobile clipboard is plain text only, excluding images/files. Remote audio, pre-login services and WinPE remain experimental.

[Validation record](https://github.com/VaderChen/YourDesk/blob/main/docs/RELEASE-VALIDATION-2026-11-05-CLIPBOARD.md)

## 日本語

- 遠隔側でコピー・切り取り後すぐに貼り付けた場合に、古いローカル内容で上書きする問題を修正しました。Win+V の履歴は貼り付け転送を開始しません。macOS は遠隔画面にフォーカスがある場合だけ所定の Control 組み合わせを取得し、ローカル編集欄のフォーカスを維持します。
- Windows 修飾キーのスキャンコード、Mac の Ctrl+Space と修飾フラグを修正しました。Android 外付けキーボードの文字・記号・候補選択スペースは遠隔 IME に送り、画面キーボードは確定文字を送ります。
- Android の前面デスクトップは双方向のプレーンテキスト（32 KiB）をサポートします。遠隔側のコピーを端末で貼り付けでき、端末の文字は遠隔貼り付け操作時に送信します。バックグラウンドでは読みません。両端を更新してください。

配布物：macOS Apple Silicon DMG、Windows x64／ARM64 Portable・サービス ZIP、Linux x64／arm64 CLI Host ZIP、実験的な WinPE x64 ZIP、[Android arm64 ZIP](https://github.com/VaderChen/YourDesk/releases/download/1.26.1105-build-2314/YourDesk-1.26.1105-build-2314-android-arm64.zip)。macOS APP と DMG は Developer ID 署名・Apple 公証済みで、公証チケットも添付済みです。画面収録とアクセシビリティの許可が必要です。Linux はデスクトップ Viewer を含みません。

Windows の `-portable.zip` を書き込み可能なフォルダーに全て展開し、DLL とライセンスを保持して `YourDesk.exe` を実行してください。WebView2 Runtime が必要です。NSIS インストーラーは提供せず、ZIP 内 EXE は信頼されたコード署名が未対応のため警告が出る場合があります。`-service.zip` は承認済みサービスの更新専用です。オンライン OTA はサービスの引き継ぎを維持します。初回導入・旧サービス移行には管理者の許可が必要で、ローカルパッケージから保護サービスを更新する際にも今回の UAC 承認が必要です。システム操作を許可すると実行中サービスが安全なデスクトップを扱え、ログイン前起動の有効化は必須ではありません。権限のない Host は対応できず、全ドライバーでの UAC 操作を実機確認済みとはしていません。旧版を手動置換する前はサービスを削除して APP を終了してください。ログイン前起動の無効化や ZIP 置換だけでは既存サービスの削除・更新は行われません。

Android 8.0 以降の arm64 スマートフォン向け Viewer です。ZIP を展開し、正式署名 APK をインストールしてください。同じ署名と applicationId を使用するため、アンインストールせず更新できます。`1.26.1003 build 2028` より前の版は一度手動更新が必要です。更新機能のある版はホーム画面が前面で待機中に検出・ダウンロードし、接続やバックグラウンド移行時に停止します。インストールにはユーザーと Android の確認が必要で、要求された場合は YourDesk からのインストールを許可してください。スマートフォンの被制御、専用ファイル転送、AV1 は Android 版の対象外です。

検証：ローカルの実 P2P 接続で異なる中国語文字列を双方向に転送し、隔離された macOS ペーストボードへの書き込みを確認しました。デスクトップキー・クリップボード、Android core、ブラウザー Smoke 33 項目を通過しました。Windows／macOS の端末間での注音切替・候補選択、Android 外付けキーボードの実機一貫検証は未完了です。モバイルはプレーンテキストのみで画像・ファイルは対象外です。遠隔音声、ログイン前サービス、WinPE は実験的です。

[検証記録](https://github.com/VaderChen/YourDesk/blob/main/docs/RELEASE-VALIDATION-2026-11-05-CLIPBOARD.md)

## 한국어

- 데스크톱에서 복사·잘라내기 직후 붙여넣을 때 오래된 로컬 내용으로 원격 클립보드를 덮어쓰는 문제를 수정했습니다. Win+V 기록은 붙여넣기 전송을 시작하지 않습니다. macOS는 원격 화면에 포커스가 있을 때 지정된 Control 조합만 캡처하고 로컬 편집 필드의 포커스를 유지합니다.
- Windows 수정 키 스캔 코드, Mac의 Ctrl+Space 및 수정 키 플래그 처리를 수정했습니다. Android 외장 키보드의 문자·기호·후보 선택 공백은 원격 IME로 보내고 화면 키보드는 확정된 문자를 보냅니다.
- Android 전경 데스크톱은 양방향 일반 텍스트 클립보드(32 KiB)를 지원합니다. 원격 복사 텍스트를 휴대폰에 붙여넣을 수 있으며 휴대폰 텍스트는 원격 붙여넣기 시 전송합니다. 백그라운드에서 읽지 않습니다. 양쪽 앱을 업데이트하세요.

배포물: macOS Apple Silicon DMG, Windows x64/ARM64 Portable·서비스 ZIP, Linux x64/arm64 CLI Host ZIP, 실험용 WinPE x64 ZIP, [Android arm64 ZIP](https://github.com/VaderChen/YourDesk/releases/download/1.26.1105-build-2314/YourDesk-1.26.1105-build-2314-android-arm64.zip). macOS 앱과 DMG 모두 Developer ID 서명, Apple 공증 및 공증 티켓 첨부를 완료했습니다. 화면 기록과 손쉬운 사용 권한이 필요합니다. Linux 패키지는 데스크톱 Viewer를 포함하지 않습니다.

Windows에서는 `-portable.zip` 전체를 쓰기 가능한 폴더에 풀고 DLL·라이선스를 유지한 채 `YourDesk.exe`를 실행하세요. WebView2 Runtime이 필요합니다. NSIS 설치 EXE 없이 ZIP만 제공하며, 내부 EXE에는 신뢰할 수 있는 코드 서명이 없어 시스템 경고가 표시될 수 있습니다. `-service.zip`은 승인된 서비스 업데이트 전용입니다. 온라인 OTA는 서비스 인계를 유지합니다. 최초 설치나 구형 서비스 이전에는 관리자 승인이 필요하고, 로컬 패키지로 보호된 서비스를 업데이트할 때도 해당 업데이트의 UAC 승인이 필요합니다. 시스템 제어를 승인하면 로그인 전 시작을 켜지 않아도 실행 중 서비스가 보안 데스크톱을 처리할 수 있습니다. 권한 없는 Host는 처리할 수 없으며, 모든 드라이버의 UAC 동작을 실기기 검증한 것은 아닙니다. 이전 설치를 수동 교체할 때는 시스템 서비스를 제거한 뒤 앱을 종료하세요. 로그인 전 시작을 끄거나 ZIP만 교체해도 기존 서비스 사본은 제거·업데이트되지 않습니다.

Android 8.0 이상 arm64 휴대폰용 Viewer입니다. ZIP을 풀고 정식 서명 APK를 설치하세요. 기존 서명과 applicationId를 유지하므로 앱 삭제 없이 업데이트할 수 있습니다. `1.26.1003 build 2028` 이전 버전은 한 번 수동 업데이트해야 합니다. 업데이트 기능이 있는 버전은 홈 화면이 전경에서 대기 중일 때 감지·다운로드하며 연결하거나 백그라운드로 이동하면 중단합니다. 설치는 사용자와 Android 확인이 필요하며, 요청 시 YourDesk를 설치 출처로 허용하세요. 휴대폰 원격 제어, 전용 파일 전송과 AV1은 Android 버전에 포함되지 않습니다.

검증: 로컬의 실제 P2P 연결에서 서로 다른 중국어 텍스트를 양방향 전송하고 격리된 macOS 클립보드 쓰기를 확인했습니다. 데스크톱 키·클립보드 테스트, Android core와 브라우저 Smoke 33개를 통과했습니다. Windows/macOS 기기 간 주음 전환과 후보 선택, Android 외장 키보드의 실기기 종단 간 검증은 완료하지 않았습니다. 모바일은 일반 텍스트만 지원하고 이미지·파일은 제외됩니다. 원격 오디오, 로그인 전 서비스 및 WinPE는 실험 기능입니다.

[검증 기록](https://github.com/VaderChen/YourDesk/blob/main/docs/RELEASE-VALIDATION-2026-11-05-CLIPBOARD.md)
