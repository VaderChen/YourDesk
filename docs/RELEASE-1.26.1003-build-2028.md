## 繁體中文

- 減少 AV1 影格驗證、終端機輸出及檔案內容搜尋的重複配置，並改善大量站台、群組與傳輸進度的更新效率；維持既有 UI、操作、功能、畫質及協定。
- Android 新增自動偵測與下載 APK 更新，並減少 AVC／HEVC 解析的完整影格複製與接收佇列搬移。首次請手動安裝本版，之後的更新仍由 Android 系統請使用者確認安裝。
- Windows x64／ARM64 持續提供 Portable ZIP 與同版同架構的服務更新 ZIP，不發布安裝 EXE。

Windows 請完整解壓縮 `-portable.zip` 到具有寫入權限的資料夾，再執行 `YourDesk.exe`，保留 DLL 與授權資料，並備妥 WebView2 Runtime。本版不提供 NSIS 安裝包；ZIP 內的 EXE 尚無可信程式碼簽章，仍可能出現系統警告。`-service.zip` 只供已授權的登入前服務更新，不是一般啟動套件。APP 內 ZIP 更新沿用服務交接；手動替換前請先退出 APP，並停用已啟用的登入前服務。舊 APP／服務首次升級可能需要手動下載或授權遷移。

套件範圍：macOS Apple Silicon 簽章／公證 DMG、Windows x64／ARM64 Portable 與服務 ZIP、Linux x64／arm64 CLI Host ZIP、WinPE x64 實驗性 ZIP，以及 [Android arm64 ZIP](https://github.com/VaderChen/YourDesk/releases/download/1.26.1003-build-2028/YourDesk-1.26.1003-build-2028-android-arm64.zip)。Android 適用 Android 8.0 以上手機 Viewer，請先解壓縮 ZIP，再安裝其中的正式簽章 APK。既有公開版尚未包含更新器，首次必須手動安裝本版；沿用相同正式簽章及 applicationId，可覆蓋更新而不需卸載。之後在首頁前景閒置時自動檢查及下載，連線或進入背景會停止工作；使用者確認後才交由 Android 安裝，必要時先允許 YourDesk 安裝來源。

驗證涵蓋完整 Go 發行旗標測試、相關競態檢查、本機 P2P／PTY 終端機、桌面四語瀏覽器操作、500 站台／40 群組、檔案傳輸及重連、套件版本／架構／雜湊、macOS 簽章／公證。Android 已通過 core 競態測試、10,620 個主機影像斷言、26 項瀏覽器回歸、25 項 JUnit、lint、Debug／Release 建置，以及正式 APK 版本、v2／v3 簽章、憑證延續性、16 KiB ELF／ZIP 和封裝雜湊檢查，但未完成本輪實機解碼與安裝更新驗收。函式量測不等同整體 RSS、FPS 或跨機延遲；Windows／WOA／Linux／WinPE 實機、Windows ZIP 更新端到端、長時間跨機傳輸及系統錄放音仍待驗收。遠端聲音、登入前服務及 WinPE 維持實驗性。

技術紀錄：[函式級最佳化](https://github.com/VaderChen/YourDesk/blob/main/docs/FUNCTION-OPTIMIZATION-2026-10-03.md)、[Android 自動更新及首次安裝條件](https://github.com/VaderChen/YourDesk/blob/main/docs/ANDROID-UPDATES.md)。

## English

- Reduce repeated allocations in AV1 frame validation, terminal output and file-content search, and improve updates of large site lists, group counts and transfer progress. Preserve the existing UI, controls, features, image quality and protocols.
- Android now detects and downloads APK updates automatically, with fewer full-frame copies in AVC/HEVC parsing and less receive-queue movement. Install this version manually once; Android will still ask you to confirm later update installations.
- Windows x64/ARM64 continue to ship Portable ZIPs and matching service update ZIPs, without installer EXEs.

Extract the entire Windows `-portable.zip` into a writable folder and run `YourDesk.exe`, keeping the DLLs and licenses together. WebView2 Runtime is required. This release has no NSIS installer; the EXEs in the ZIP are not signed with a trusted code-signing certificate and may trigger system warnings. The `-service.zip` is only for updating an authorized pre-login service, not for launching the app. In-app ZIP updates use the service handoff. Quit the app and disable an enabled pre-login service before replacing files manually. Older apps or services may need a manual download or authorized migration on their first upgrade.

Packages: signed/notarized macOS Apple Silicon DMG; Windows x64/ARM64 Portable and service ZIPs; Linux x64/arm64 CLI Host ZIPs; experimental WinPE x64 ZIP; and [Android arm64 ZIP](https://github.com/VaderChen/YourDesk/releases/download/1.26.1003-build-2028/YourDesk-1.26.1003-build-2028-android-arm64.zip). Android is a phone Viewer for arm64 devices running Android 8.0 or later. Extract the ZIP, then install the officially signed APK. Previously published versions lack the updater, so install this version manually once. It uses the same official signing certificate and applicationId and supports updating without uninstalling. Later releases are checked and downloaded while the home screen is idle in the foreground; connecting or moving to the background stops update work. Android installs only after user confirmation; allow YourDesk as an installation source if requested.

Validation covers the full Go suite with release build tags, relevant race checks, local P2P/PTY terminal sessions, desktop browser flows in four languages, 500 sites/40 groups, file transfers/reconnection, package versions/architectures/hashes and macOS signing/notarization. Android passed core race checks, 10,620 host video assertions, 26 browser cases, 25 JUnit tests, lint, Debug/Release builds and checks of the official APK version, v2/v3 signatures, signing-certificate continuity, 16 KiB ELF/ZIP alignment and package hashes. This round did not validate decoding or update installation on a physical Android device. Function benchmarks do not establish whole-process RSS, FPS or cross-device latency improvements. Physical Windows/WOA/Linux/WinPE devices, end-to-end Windows ZIP updates, sustained cross-device transfers and system audio capture/playback still need validation. Remote audio, pre-login services and WinPE remain experimental.

Technical records: [function-level optimization](https://github.com/VaderChen/YourDesk/blob/main/docs/FUNCTION-OPTIMIZATION-2026-10-03.md) and [Android updates and initial installation requirements](https://github.com/VaderChen/YourDesk/blob/main/docs/ANDROID-UPDATES.md).

## 日本語

- AV1 フレーム検証、ターミナル出力、ファイル内容検索の重複割り当てを削減し、多数の接続先・グループ件数・転送進捗の更新を効率化しました。既存の UI、操作、機能、画質、プロトコルを維持します。
- Android に APK 更新の自動検出・ダウンロードを追加し、AVC／HEVC 解析のフレーム全体のコピーと受信キューの移動を削減しました。最初に本版を手動インストールしてください。以後の更新も、インストール時は Android でユーザーの確認が必要です。
- Windows x64／ARM64 は引き続き Portable ZIP と同版・同アーキテクチャのサービス更新 ZIP を提供し、インストーラー EXE は配布しません。

Windows の `-portable.zip` を書き込み可能なフォルダーに全て展開し、DLL とライセンスを保持して `YourDesk.exe` を実行してください。WebView2 Runtime が必要です。NSIS インストーラーは提供しません。ZIP 内の EXE は信頼されたコード署名に未対応で、システムの警告が表示される場合があります。`-service.zip` は承認済みのログイン前サービスの更新専用で、通常の起動用ではありません。アプリ内の ZIP 更新はサービスの引き継ぎを使用します。手動でファイルを置き換える前にアプリを終了し、有効なログイン前サービスを無効にしてください。旧アプリ・サービスの初回更新には手動ダウンロードや移行の承認が必要な場合があります。

配布物：署名・公証済み macOS Apple Silicon DMG、Windows x64／ARM64 Portable／サービス ZIP、Linux x64／arm64 CLI Host ZIP、実験的な WinPE x64 ZIP、[Android arm64 ZIP](https://github.com/VaderChen/YourDesk/releases/download/1.26.1003-build-2028/YourDesk-1.26.1003-build-2028-android-arm64.zip)。Android 8.0 以降の arm64 スマートフォン向け Viewer です。ZIP を展開し、中の正式署名 APK をインストールしてください。従来の公開版には更新機能がないため、最初に本版を一度手動インストールします。同じ正式署名と applicationId を使用し、アンインストールせずに更新できます。その後はホーム画面が前面で待機中に更新を検出・ダウンロードし、接続時やバックグラウンド移行時には停止します。インストールは Android での確認後に実行され、必要な場合は YourDesk からのインストールを許可してください。

検証範囲は正式ビルドタグでの Go 全テスト、関連する race 検査、ローカル P2P／PTY ターミナル、デスクトップの四言語ブラウザー操作、500 接続先／40 グループ、ファイル転送と再接続、パッケージのバージョン・アーキテクチャ・ハッシュ、macOS の署名と公証です。Android は core の race 検査、ホスト映像テスト 10,620 アサーション、ブラウザー 26 件、JUnit 25 件、lint、Debug／Release ビルドに加え、正式 APK のバージョン、v2／v3 署名、署名証明書の継続性、16 KiB ELF／ZIP 配置、パッケージハッシュの検査を通過しました。今回は Android 実機でのデコードと更新インストールは未検証です。関数の測定値はプロセス全体の RSS、FPS、実機間遅延の改善を示しません。Windows／WOA／Linux／WinPE 実機、Windows ZIP 更新全体、長時間の実機間転送、システム音声入出力は引き続き検証が必要です。音声、ログイン前サービス、WinPE は実験的機能です。

技術記録：[関数単位の最適化](https://github.com/VaderChen/YourDesk/blob/main/docs/FUNCTION-OPTIMIZATION-2026-10-03.md)、[Android 自動更新と初回インストール条件](https://github.com/VaderChen/YourDesk/blob/main/docs/ANDROID-UPDATES.md)。

## 한국어

- AV1 프레임 검증, 터미널 출력과 파일 내용 검색의 반복 할당을 줄이고 대량 접속 대상, 그룹별 개수와 전송 진행률 갱신을 개선했습니다. 기존 UI, 조작, 기능, 화질과 프로토콜은 유지합니다.
- Android에 APK 업데이트 자동 감지·다운로드를 추가하고 AVC/HEVC 분석의 전체 프레임 복사와 수신 큐 이동을 줄였습니다. 이번 버전을 먼저 한 번 수동 설치하세요. 이후 업데이트 설치도 Android에서 사용자가 확인해야 합니다.
- Windows x64/ARM64는 계속 Portable ZIP과 같은 버전·아키텍처의 서비스 업데이트 ZIP을 제공하며 설치용 EXE는 배포하지 않습니다.

Windows `-portable.zip`을 쓰기 가능한 폴더에 모두 압축 해제하고 DLL 및 라이선스를 유지한 상태로 `YourDesk.exe`를 실행하세요. WebView2 Runtime이 필요합니다. NSIS 설치 프로그램은 제공하지 않습니다. ZIP 안의 EXE는 신뢰된 코드 서명이 없어 시스템 경고가 나타날 수 있습니다. `-service.zip`은 승인된 로그인 전 서비스의 업데이트 전용이며 일반 앱 실행용이 아닙니다. 앱 내 ZIP 업데이트는 서비스 인계를 사용합니다. 파일을 수동으로 교체하기 전에 앱을 종료하고 활성화된 로그인 전 서비스도 꺼 주세요. 구형 앱이나 서비스의 첫 업그레이드에는 수동 다운로드 또는 이전 승인이 필요할 수 있습니다.

배포 패키지: 서명/공증된 macOS Apple Silicon DMG, Windows x64/ARM64 Portable/서비스 ZIP, Linux x64/arm64 CLI Host ZIP, 실험용 WinPE x64 ZIP 및 [Android arm64 ZIP](https://github.com/VaderChen/YourDesk/releases/download/1.26.1003-build-2028/YourDesk-1.26.1003-build-2028-android-arm64.zip). Android 8.0 이상 arm64 휴대폰용 Viewer입니다. ZIP을 먼저 푼 뒤 포함된 정식 서명 APK를 설치하세요. 이전 공개 버전에는 업데이트 기능이 없으므로 이번 버전을 한 번 수동 설치해야 합니다. 같은 정식 서명과 applicationId를 사용하므로 앱을 삭제하지 않고 업데이트할 수 있습니다. 이후에는 홈 화면이 전경에서 대기 중일 때 업데이트를 감지하고 다운로드하며, 연결하거나 백그라운드로 이동하면 작업을 중단합니다. 설치는 Android에서 사용자가 확인한 뒤 진행되며, 요청 시 YourDesk를 설치 출처로 허용해야 합니다.

검증 범위는 정식 빌드 태그의 전체 Go 테스트, 관련 race 검사, 로컬 P2P/PTY 터미널, 데스크톱 4개 언어 브라우저 조작, 접속 대상 500개/그룹 40개, 파일 전송과 재연결, 패키지 버전·아키텍처·해시 및 macOS 서명/공증입니다. Android는 core race 검사, 호스트 영상 검증 10,620개, 브라우저 사례 26개, JUnit 25개, lint, Debug/Release 빌드와 정식 APK 버전, v2/v3 서명, 서명 인증서 연속성, 16 KiB ELF/ZIP 정렬 및 패키지 해시 검사를 통과했습니다. 이번에는 Android 실기기의 디코딩과 업데이트 설치를 검증하지 않았습니다. 함수 벤치마크는 전체 프로세스 RSS, FPS 또는 기기 간 지연 개선을 의미하지 않습니다. Windows/WOA/Linux/WinPE 실기기, Windows ZIP 업데이트 전체 과정, 장시간 기기 간 전송 및 시스템 오디오 입출력은 추가 검증이 필요합니다. 원격 오디오, 로그인 전 서비스와 WinPE는 실험 기능입니다.

기술 기록: [함수 단위 최적화](https://github.com/VaderChen/YourDesk/blob/main/docs/FUNCTION-OPTIMIZATION-2026-10-03.md), [Android 자동 업데이트 및 최초 설치 조건](https://github.com/VaderChen/YourDesk/blob/main/docs/ANDROID-UPDATES.md).
