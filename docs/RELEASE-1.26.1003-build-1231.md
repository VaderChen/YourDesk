## 繁體中文

- 減少遠端畫面、補幀、聲音及傳輸路徑的重複配置與記憶體保留，維持原有 UI、操作、功能、畫質及協定。
- 改善檔案傳輸取消後的暫存清理與重試、斷線後的手動恢復，以及大量檔案清單的處理效率。
- Windows x64／ARM64 持續使用 Portable ZIP 發布與自動更新，並提供同版同架構的服務更新 ZIP；修正套件內仍指向 setup.exe 的舊說明。

Windows 請完整解壓縮 `-portable.zip` 到具有寫入權限的資料夾，再執行 `YourDesk.exe`，保留 DLL 與授權資料，並備妥 WebView2 Runtime。本版不提供 NSIS 安裝包；ZIP 內的 EXE 尚無可信程式碼簽章，仍可能出現系統警告。`-service.zip` 只供已授權的登入前服務更新，不是一般啟動套件。APP 內 ZIP 更新沿用服務交接；手動替換前請先退出 APP，並停用已啟用的登入前服務。舊 APP／服務首次升級可能需要手動下載或授權遷移。

套件範圍：macOS Apple Silicon 簽章／公證 DMG、Windows x64／ARM64 Portable 與服務 ZIP、Linux x64／arm64 CLI Host ZIP，以及 WinPE x64 實驗性 ZIP。Android 本次未重建，請使用既有的 [Android ZIP](https://github.com/VaderChen/YourDesk/releases/download/1.26.1002-build-2301/YourDesk-1.26.1002-build-2301-android-arm64.zip)，解壓後安裝 APK；Android 仍為手機 Viewer，適用 Android 8.0 以上 arm64。

驗證涵蓋完整 Go 測試、相關套件 race 檢查、檔案視窗瀏覽器 smoke、Apple 原生補幀呈現／雙視窗切換、ZIP 封裝、套件選擇與解壓檢查、套件版本／架構／雜湊，以及 macOS 簽章與公證。配置量測屬於特定 Go 路徑，不能直接換算為整體 RSS、FPS 或跨機延遲。Windows／WOA／Linux／WinPE 實機、Windows ZIP 更新端到端、跨機長時間傳輸及真實系統錄放音仍待驗收；遠端聲音、登入前服務與 WinPE 維持實驗性。

技術紀錄：[第一輪效能](https://github.com/VaderChen/YourDesk/blob/main/docs/PERFORMANCE-OPTIMIZATION-2026-10-03.md)、[檔案傳輸穩定性](https://github.com/VaderChen/YourDesk/blob/main/docs/FILE-TRANSFER-STABILITY-2026-10-03.md)、[第二輪效能](https://github.com/VaderChen/YourDesk/blob/main/docs/PERFORMANCE-OPTIMIZATION-2026-10-03-ROUND-2.md)。

## English

- Reduce repeated allocations and retained memory in remote video, frame interpolation, audio and transfer paths while preserving the UI, controls, features, image quality and protocols.
- Improve cleanup and retries after transfer cancellation, manual recovery after disconnection and processing of large file listings.
- Windows x64 and ARM64 continue to ship and update through Portable ZIP, with matching service update ZIPs. Correct outdated setup.exe instructions in the package documentation.

Extract the entire Windows `-portable.zip` into a writable folder and run `YourDesk.exe`, keeping the DLLs and licenses together. WebView2 Runtime is required. This release has no NSIS installer; the EXEs in the ZIP are not signed with a trusted code-signing certificate and may trigger system warnings. The `-service.zip` is only for updating an authorized pre-login service, not for launching the app. In-app ZIP updates use the service handoff. Quit the app and disable an enabled pre-login service before replacing files manually. Older apps or services may need a manual download or authorized migration on their first upgrade.

Packages: signed/notarized macOS Apple Silicon DMG; Windows x64/ARM64 Portable and service ZIPs; Linux x64/arm64 CLI Host ZIPs; and an experimental WinPE x64 ZIP. Android was not rebuilt; use the existing [Android ZIP](https://github.com/VaderChen/YourDesk/releases/download/1.26.1002-build-2301/YourDesk-1.26.1002-build-2301-android-arm64.zip), extract it, then install the APK. Android remains a phone Viewer for arm64 devices running Android 8.0 or later.

Validation covers the full Go test suite, race checks for affected packages, file-window browser smoke tests, native Apple frame presentation and two-window switching, ZIP packaging, package selection and extraction checks, package versions/architectures/hashes, and macOS signing/notarization. Allocation measurements apply to specific Go paths and do not establish whole-process RSS, FPS or cross-device latency improvements. Physical Windows/WOA/Linux/WinPE devices, end-to-end Windows ZIP updates, sustained cross-device transfers and real system audio capture/playback still need validation. Remote audio, pre-login services and WinPE remain experimental.

Technical records: [performance round one](https://github.com/VaderChen/YourDesk/blob/main/docs/PERFORMANCE-OPTIMIZATION-2026-10-03.md), [file-transfer stability](https://github.com/VaderChen/YourDesk/blob/main/docs/FILE-TRANSFER-STABILITY-2026-10-03.md), and [performance round two](https://github.com/VaderChen/YourDesk/blob/main/docs/PERFORMANCE-OPTIMIZATION-2026-10-03-ROUND-2.md).

## 日本語

- 遠隔画面・フレーム補間・音声・転送処理の重複割り当てとメモリー保持を削減し、UI、操作方法、機能、画質、プロトコルを維持します。
- ファイル転送のキャンセル後の後処理と再試行、切断後の手動再開、大量のファイル一覧処理を改善します。
- Windows x64／ARM64 は引き続き Portable ZIP で配布・更新し、同版・同アーキテクチャのサービス更新 ZIP も提供します。setup.exe を案内していた古い説明を修正しました。

Windows の `-portable.zip` を書き込み可能なフォルダーに全て展開し、DLL とライセンスを保持して `YourDesk.exe` を実行してください。WebView2 Runtime が必要です。NSIS インストーラーは提供しません。ZIP 内の EXE は信頼されたコード署名に未対応で、システムの警告が表示される場合があります。`-service.zip` は承認済みのログイン前サービスの更新専用で、通常の起動用ではありません。アプリ内の ZIP 更新はサービスの引き継ぎを使用します。手動でファイルを置き換える前にアプリを終了し、有効なログイン前サービスを無効にしてください。旧アプリ・サービスの初回更新には手動ダウンロードや移行の承認が必要な場合があります。

配布物：署名・公証済み macOS Apple Silicon DMG、Windows x64／ARM64 Portable／サービス ZIP、Linux x64／arm64 CLI Host ZIP、実験的な WinPE x64 ZIP。Android は再ビルドしていません。既存の [Android ZIP](https://github.com/VaderChen/YourDesk/releases/download/1.26.1002-build-2301/YourDesk-1.26.1002-build-2301-android-arm64.zip)を展開し、APK をインストールしてください。Android 8.0 以降の arm64 スマートフォン向け Viewer です。

検証範囲は Go 全テスト、関連パッケージの race 検査、ファイル画面のブラウザー smoke、Apple ネイティブ補間の表示と二つのウィンドウの切り替え、ZIP パッケージの作成・選択・展開検査、バージョン・アーキテクチャ・ハッシュ、macOS の署名と公証です。割り当て量は特定の Go 処理の測定値であり、プロセス全体の RSS、FPS、実機間の遅延改善を示すものではありません。Windows／WOA／Linux／WinPE 実機、Windows ZIP 更新全体、長時間の実機間転送、実際のシステム音声入出力は引き続き検証が必要です。音声、ログイン前サービス、WinPE は実験的機能です。

技術記録：[最適化第1回](https://github.com/VaderChen/YourDesk/blob/main/docs/PERFORMANCE-OPTIMIZATION-2026-10-03.md)、[ファイル転送の安定性](https://github.com/VaderChen/YourDesk/blob/main/docs/FILE-TRANSFER-STABILITY-2026-10-03.md)、[最適化第2回](https://github.com/VaderChen/YourDesk/blob/main/docs/PERFORMANCE-OPTIMIZATION-2026-10-03-ROUND-2.md)。

## 한국어

- 원격 화면, 프레임 보간, 오디오와 전송 처리의 반복 할당 및 메모리 보유를 줄이면서 UI, 조작, 기능, 화질과 프로토콜을 유지합니다.
- 파일 전송 취소 후 정리와 재시도, 연결 해제 후 수동 복구 및 대량 파일 목록 처리를 개선합니다.
- Windows x64／ARM64는 계속 Portable ZIP으로 배포하고 업데이트하며, 같은 버전과 아키텍처의 서비스 업데이트 ZIP도 제공합니다. setup.exe를 안내하던 오래된 설명을 수정했습니다.

Windows `-portable.zip`을 쓰기 가능한 폴더에 모두 압축 해제하고 DLL 및 라이선스를 유지한 상태로 `YourDesk.exe`를 실행하세요. WebView2 Runtime이 필요합니다. NSIS 설치 프로그램은 제공하지 않습니다. ZIP 안의 EXE는 신뢰된 코드 서명이 없어 시스템 경고가 나타날 수 있습니다. `-service.zip`은 승인된 로그인 전 서비스의 업데이트 전용이며 일반 앱 실행용이 아닙니다. 앱 내 ZIP 업데이트는 서비스 인계를 사용합니다. 파일을 수동으로 교체하기 전에 앱을 종료하고 활성화된 로그인 전 서비스도 꺼 주세요. 구형 앱이나 서비스의 첫 업그레이드에는 수동 다운로드 또는 이전 승인이 필요할 수 있습니다.

배포 패키지: 서명／공증된 macOS Apple Silicon DMG, Windows x64／ARM64 Portable／서비스 ZIP, Linux x64／arm64 CLI Host ZIP 및 실험용 WinPE x64 ZIP. Android는 다시 빌드하지 않았습니다. 기존 [Android ZIP](https://github.com/VaderChen/YourDesk/releases/download/1.26.1002-build-2301/YourDesk-1.26.1002-build-2301-android-arm64.zip)을 압축 해제한 뒤 APK를 설치하세요. Android 8.0 이상 arm64 휴대폰용 Viewer입니다.

검증 범위는 전체 Go 테스트, 관련 패키지의 race 검사, 파일 창 브라우저 smoke, Apple 네이티브 보간 표시와 두 창 전환, ZIP 패키징, 패키지 선택 및 압축 해제 검사, 버전／아키텍처／해시, macOS 서명과 공증입니다. 할당량은 특정 Go 경로의 측정값이며 전체 프로세스 RSS, FPS 또는 기기 간 지연 개선을 나타내지 않습니다. Windows／WOA／Linux／WinPE 실기기, Windows ZIP 업데이트 전체 과정, 장시간 기기 간 전송 및 실제 시스템 오디오 입출력은 추가 검증이 필요합니다. 원격 오디오, 로그인 전 서비스 및 WinPE는 실험 기능입니다.

기술 기록: [1차 성능 최적화](https://github.com/VaderChen/YourDesk/blob/main/docs/PERFORMANCE-OPTIMIZATION-2026-10-03.md), [파일 전송 안정성](https://github.com/VaderChen/YourDesk/blob/main/docs/FILE-TRANSFER-STABILITY-2026-10-03.md), [2차 성능 최적화](https://github.com/VaderChen/YourDesk/blob/main/docs/PERFORMANCE-OPTIMIZATION-2026-10-03-ROUND-2.md).
