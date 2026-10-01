## 繁體中文

適用 Android 8.0 以上的 arm64 裝置；此 APK 是操作遠端電腦的 Viewer。

- 補齊雙指縮放／拖移、長按右鍵、遠端捲動與多螢幕切換；切換期間等候確認及新畫面再恢復控制。
- 加入遠端聲音 Opus／AAC／PCM 協商與播放、站台管理、加密記憶密碼、QR 匯入及 Shell。遠端聲音預設關閉，可由喇叭按鈕開啟。
- 修正串流開始、旋轉及重連後關閉按鈕偏移；已在 Android 11 安裝正式 APK，實際連線並以 ADB 擷圖確認右上角位置。

### 下載與安裝

下載附件 `YourDesk-1.26.1001-build-1524-android-arm64.zip`，解壓縮後開啟其中的 APK 安裝；若系統詢問，允許此次使用的檔案管理器安裝 App。正式套件為 `com.yourdesk.android`，已完成非 Debug、v2／v3 簽章及全部原生庫 16 KB ELF／ZIP 靜態檢查。ZIP 內含四語安裝說明、授權文件、BUILD.json 與 SHA256SUMS，另附 `.zip.sha256` 及 `.zip.json` 校驗 ZIP；靜態對齊檢查不等於 16 KB 裝置實測。

舊 `0.1.0` 測試版使用不同簽章，不能直接覆蓋；持有原簽章金鑰時，可在支援的 Android 版本進行簽章遷移。本次已在 Android 11 保留原站台與記憶密碼升級成功。卸載會清除 App 資料，請先保存站台資訊並確認可重新取得密碼，再參閱[安裝及升級說明](https://github.com/VaderChen/YourDesk/blob/main/docs/ANDROID-RELEASE-2026-10-01.md#安裝與升級)。

### 驗證與限制

功能整合階段（build 1449）的 37 項裝置／Wi-Fi Smoke 全部通過；關閉按鈕修正後（build 1524）另完成 17 項畫面／操作 Smoke 及截圖像素檢查。發布前再核對 25 項前端 Smoke、幾何／音訊政策檢查及 APK 簽章。這些是不同階段的結果，不代表在最終 APK 重跑所有測試。Release lint 為零 error／fatal，仍有 30 項 warning。[完整功能與分階段驗證紀錄](https://github.com/VaderChen/YourDesk/blob/main/docs/ANDROID-RELEASE-2026-10-01.md)。

手機版尚未提供剪貼簿同步、獨立檔案傳輸、AV1 或手機被控模式。Android 16／16 KB 裝置、HEVC 與跨廠解碼器、平板／折疊螢幕、真實通話／耳機／藍牙切換、換網及長時間負載仍待對應環境驗收。此發行提供內含 APK 的 ZIP 下載，未提交 Google Play。

APK SHA-256：`142e922d0cf4ca906f10262b819d55031c753ef787f7c7bdfea30f1898cd908a`。

## English

For arm64 devices running Android 8.0 or later. This APK is a viewer for controlling a remote computer.

- Adds pinch zoom and two-finger panning, long-press right-click, remote scrolling, and switching between monitors. Control resumes after the host acknowledges the selected monitor and its new image arrives.
- Adds remote audio negotiation and playback with Opus/AAC/PCM, saved-site management, encrypted password storage, QR import, and Shell access. Remote audio is off by default; use the speaker button to enable it.
- Fixes the close button shifting after streaming starts, screen rotation, or reconnection. The signed release APK was installed on Android 11 and connected to a real host; ADB screenshots confirmed the button's position at the upper right.

### Download and installation

Download the attached `YourDesk-1.26.1001-build-1524-android-arm64.zip`, extract it, then open the included APK to install it. If prompted, allow the file manager you are using to install apps. The package ID is `com.yourdesk.android`. The APK is not debuggable and passed v2/v3 signature verification and static 16 KB ELF/ZIP alignment checks for all native libraries. The ZIP includes installation instructions in four languages, license files, BUILD.json, and SHA256SUMS. The attached `.zip.sha256` and `.zip.json` verify the ZIP itself. Static alignment checks do not replace testing on a device with 16 KB memory pages.

The older `0.1.0` test build uses a different signing key and cannot be updated directly with this APK. If the original signing key is available, signing-key migration is possible on supported Android versions. Migration was verified on Android 11 while preserving saved sites and passwords. Uninstalling clears app data; save your site details and ensure you can recover the passwords first. See the [installation and upgrade instructions (Traditional Chinese)](https://github.com/VaderChen/YourDesk/blob/main/docs/ANDROID-RELEASE-2026-10-01.md#安裝與升級).

### Validation and limitations

All 37 device/Wi-Fi smoke tests passed during feature integration (build 1449). After the close-button fix (build 1524), 17 UI/interaction smoke tests and screenshot pixel checks passed. Before publication, 25 frontend smoke cases, geometry/audio policy checks, and the APK signature were checked again. These results come from separate stages; the entire suite was not rerun on the final APK. Release lint reported no errors or fatal issues and 30 warnings. See the [full feature list and validation record by stage (Traditional Chinese)](https://github.com/VaderChen/YourDesk/blob/main/docs/ANDROID-RELEASE-2026-10-01.md).

The Android app does not yet provide clipboard synchronization, a dedicated file-transfer interface, AV1, or remote control of the phone itself. Android 16, devices with 16 KB memory pages, HEVC and different vendors' decoders, tablets/foldables, actual phone calls and headset/Bluetooth switching, network handovers, and sustained workloads still require validation in their respective environments. This release is available as a ZIP containing the APK and has not been submitted to Google Play.

APK SHA-256: `142e922d0cf4ca906f10262b819d55031c753ef787f7c7bdfea30f1898cd908a`.

## 日本語

Android 8.0 以降の arm64 端末に対応する、リモートコンピューターを操作するための Viewer です。

- ピンチによる拡大・縮小、2 本指での表示範囲の移動、長押しによる右クリック、リモートスクロール、モニター切り替えに対応しました。切り替え時は、ホストの確認応答と新しい画面の受信を待ってから操作を再開します。
- Opus／AAC／PCM によるリモート音声の形式交渉と再生、接続先管理、パスワードの暗号化保存、QR インポート、Shell 接続を追加しました。リモート音声は初期設定でオフです。スピーカーボタンで有効にできます。
- ストリーミング開始、画面回転、再接続後に閉じるボタンがずれる問題を修正しました。署名済みの正式 APK を Android 11 にインストールし、実際のホストに接続して、ADB スクリーンショットで右上の位置を確認しました。

### ダウンロードとインストール

添付の `YourDesk-1.26.1001-build-1524-android-arm64.zip` をダウンロードして展開し、中の APK を開いてインストールしてください。確認が表示された場合は、使用中のファイルマネージャーからのインストールを許可してください。パッケージ ID は `com.yourdesk.android` です。APK はデバッグ不可で、v2／v3 署名検証と、すべてのネイティブライブラリの 16 KB ELF／ZIP アラインメントの静的検査に合格しています。ZIP には 4 言語のインストール手順、ライセンス文書、BUILD.json、SHA256SUMS を同梱しています。添付の `.zip.sha256` と `.zip.json` で ZIP 自体を検証できます。静的検査は、メモリページサイズが 16 KB の実機での検証を代替するものではありません。

旧テスト版 `0.1.0` は署名鍵が異なるため、この APK では直接上書き更新できません。元の署名鍵があれば、対応する Android バージョンで署名鍵の移行が可能です。今回は Android 11 で、保存済みの接続先とパスワードを保持した更新を確認しました。アンインストールするとアプリのデータが消去されます。先に接続先情報を保存し、パスワードを再取得できることを確認してください。[インストールと更新の説明（繁体字中国語）](https://github.com/VaderChen/YourDesk/blob/main/docs/ANDROID-RELEASE-2026-10-01.md#安裝與升級)も参照してください。

### 検証と制限事項

機能統合段階（build 1449）の実機／Wi-Fi スモークテスト 37 項目はすべて合格しました。閉じるボタンの修正後（build 1524）には、画面・操作のスモークテスト 17 項目とスクリーンショットのピクセル検査に合格しました。公開前に、フロントエンドのスモークテスト 25 項目、表示領域の幾何計算・音声処理ポリシーの検査、APK 署名を再確認しました。これらは別々の段階の結果であり、最終 APK で全テストを再実行したものではありません。Release lint は error／fatal が 0 件、warning が 30 件です。[機能一覧と段階別の検証記録（繁体字中国語）](https://github.com/VaderChen/YourDesk/blob/main/docs/ANDROID-RELEASE-2026-10-01.md)を参照してください。

Android 版は、クリップボード同期、専用のファイル転送機能、AV1、スマートフォン自体を遠隔操作する機能には未対応です。Android 16、16 KB ページの端末、HEVC と各メーカーのデコーダー、タブレット／折りたたみ端末、実際の通話・ヘッドセット／Bluetooth の切り替え、ネットワーク切り替え、長時間の負荷については、それぞれの環境で追加検証が必要です。本リリースは APK を含む ZIP で提供し、Google Play には提出していません。

APK SHA-256：`142e922d0cf4ca906f10262b819d55031c753ef787f7c7bdfea30f1898cd908a`。

## 한국어

Android 8.0 이상을 사용하는 arm64 기기용으로, 원격 컴퓨터를 제어하는 Viewer입니다.

- 두 손가락 확대·축소 및 화면 이동, 길게 눌러 오른쪽 클릭, 원격 스크롤, 모니터 전환을 지원합니다. 모니터를 전환할 때는 호스트의 확인 응답과 새 화면을 받은 뒤 제어를 재개합니다.
- Opus/AAC/PCM 원격 오디오 형식 협상 및 재생, 접속 대상 관리, 암호화된 비밀번호 저장, QR 가져오기, Shell 접속을 추가했습니다. 원격 오디오는 기본적으로 꺼져 있으며 스피커 버튼으로 켤 수 있습니다.
- 스트리밍 시작, 화면 회전, 재연결 후 닫기 버튼의 위치가 어긋나는 문제를 수정했습니다. 서명된 정식 APK를 Android 11에 설치하고 실제 호스트에 연결한 뒤, ADB 스크린샷으로 오른쪽 위 위치를 확인했습니다.

### 다운로드 및 설치

첨부된 `YourDesk-1.26.1001-build-1524-android-arm64.zip`를 다운로드하고 압축을 푼 뒤, 포함된 APK를 열어 설치하세요. 시스템에서 요청하면 사용 중인 파일 관리자의 앱 설치를 허용하세요. 패키지 ID는 `com.yourdesk.android`입니다. APK는 디버깅이 비활성화되어 있으며 v2/v3 서명 검증과 모든 네이티브 라이브러리의 16 KB ELF/ZIP 정렬 정적 검사를 통과했습니다. ZIP에는 4개 언어의 설치 안내, 라이선스 파일, BUILD.json, SHA256SUMS가 포함되어 있습니다. 첨부된 `.zip.sha256`과 `.zip.json`으로 ZIP 자체를 검증할 수 있습니다. 정적 정렬 검사가 16 KB 메모리 페이지를 사용하는 실제 기기의 검증을 대신하지는 않습니다.

이전 `0.1.0` 테스트 버전은 서명 키가 달라 이 APK로 바로 덮어써서 업데이트할 수 없습니다. 원래 서명 키가 있으면 지원되는 Android 버전에서 서명 키를 이전할 수 있습니다. 이번에는 Android 11에서 저장된 접속 대상과 비밀번호를 유지한 채 업데이트하는 데 성공했습니다. 앱을 삭제하면 앱 데이터가 지워집니다. 먼저 접속 정보를 보관하고 비밀번호를 다시 확보할 수 있는지 확인하세요. [설치 및 업그레이드 안내(중국어 번체)](https://github.com/VaderChen/YourDesk/blob/main/docs/ANDROID-RELEASE-2026-10-01.md#安裝與升級)를 참고하세요.

### 검증 및 제한 사항

기능 통합 단계(build 1449)의 기기/Wi-Fi 스모크 테스트 37개가 모두 통과했습니다. 닫기 버튼 수정 후(build 1524)에는 화면·조작 스모크 테스트 17개와 스크린샷 픽셀 검사를 통과했습니다. 공개 전에 프런트엔드 스모크 테스트 25개, 표시 영역의 기하 계산·오디오 처리 정책 검사, APK 서명을 다시 확인했습니다. 서로 다른 단계에서 얻은 결과이며 최종 APK에서 전체 테스트를 다시 실행한 것은 아닙니다. Release lint 결과는 error/fatal 0개, warning 30개입니다. [전체 기능 및 단계별 검증 기록(중국어 번체)](https://github.com/VaderChen/YourDesk/blob/main/docs/ANDROID-RELEASE-2026-10-01.md)을 참고하세요.

Android 앱은 아직 클립보드 동기화, 별도의 파일 전송 기능, AV1, 휴대폰 자체를 원격 제어하는 기능을 제공하지 않습니다. Android 16, 16 KB 페이지 기기, HEVC 및 제조사별 디코더, 태블릿/폴더블 기기, 실제 통화와 헤드셋/Bluetooth 전환, 네트워크 전환, 장시간 부하는 각 환경에서 추가 검증이 필요합니다. 이번 릴리스는 APK가 포함된 ZIP으로 제공하며 Google Play에는 제출하지 않았습니다.

APK SHA-256: `142e922d0cf4ca906f10262b819d55031c753ef787f7c7bdfea30f1898cd908a`.
