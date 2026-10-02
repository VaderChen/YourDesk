## 繁體中文

- Windows x64／ARM64 改提供 Portable ZIP，完整解壓縮後執行 YourDesk.exe。
- Portable ZIP 更新自動下載、驗證、解壓替換並重啟；先備份，失敗時嘗試還原，保留服務更新專用套件。
- 同步提供 Android arm64 正式簽章 ZIP，解壓後安裝 APK；保留既有觸控、聲音與多螢幕功能。

Windows 暫不提供 NSIS 安裝程式；ZIP 內的 Windows EXE 尚未具有可信程式碼簽章，仍可能出現系統警告。新版 APP 自動更新 ZIP，登入前服務沿用服務交接；舊 APP／舊服務首次升級可能需手動處理，`-service.zip` 不是一般安裝包。舊桌面版可能無法辨識新的 Portable 檔名，請由本頁手動下載。macOS 提供已簽章／公證 DMG，Linux 提供 CLI Host ZIP，WinPE 維持實驗性。

Android 適用 Android 8.0 以上 arm64，是操作遠端電腦的 Viewer；下載 Android ZIP、解壓後安裝其中的 APK。沿用正式發行金鑰及遞增版本碼，既有同簽章正式版可覆蓋升級。舊 0.1.0 測試版使用不同簽章，須依[遷移說明](https://github.com/VaderChen/YourDesk/blob/main/docs/ANDROID-RELEASE-2026-10-01.md#安裝與升級)處理；勿直接卸載以免遺失站台與密碼。手機版尚未提供剪貼簿同步、獨立檔案傳輸、AV1 或手機被控模式。

本次為 ZIP 自動更新、封裝政策及統一版本發行，Android 未新增功能。驗證範圍為封裝／更新 Smoke、Android 前端與政策檢查、Release lint、APK 版本／正式 v2／v3 簽章及 16 KB ELF／ZIP 靜態檢查。先前 Android 11 的實機結果屬於舊版，本次未宣稱在新 APK 重跑全部裝置測試。Android 16／16 KB 裝置、跨廠解碼器、通話／藍牙、換網與長時間負載仍待實機驗證。Windows ZIP 更新仍待 Windows 實機端到端驗證。遠端聲音與登入前服務維持實驗性。

## English

- Windows x64 and ARM64 now ship as Portable ZIP packages; extract all files and run YourDesk.exe.
- Portable ZIP updates download, verify, back up, extract, replace and restart automatically, with rollback on failure; service-update packages remain available.
- Includes a signed Android arm64 release ZIP. Extract it and install the APK; existing touch, audio and multi-display features are retained.

NSIS installers are temporarily omitted. Windows executables in the ZIP are not yet signed with a trusted code-signing certificate and may still trigger security warnings. New versions update ZIP automatically and hand off pre-login services. Older apps/services may require a manual first upgrade. The service ZIP is not a general installer. Older desktop versions may not recognize the Portable filename; download it manually here. macOS uses signed/notarized DMG, Linux uses CLI Host ZIP, and WinPE remains experimental.

Android requires Android 8.0+ on arm64 and acts as a remote-computer Viewer. Extract the Android ZIP and install its APK. The existing release key is retained and versionCode increased; releases signed with the same key support an in-place upgrade. The old 0.1.0 test app used another key: follow the [migration guidance](https://github.com/VaderChen/YourDesk/blob/main/docs/ANDROID-RELEASE-2026-10-01.md) rather than uninstalling and losing saved data. Clipboard synchronization, dedicated file transfer, AV1 and phone hosting are not available.

This release changes packaging and aligns versions; Android has no new features. Validation covers packaging/update smoke tests, Android frontend and policy checks, Release lint, APK version/release v2/v3 signatures and static 16 KB ELF/ZIP checks. Previous Android 11 device results apply to earlier builds; all device tests were not repeated on this APK. Android 16/16 KB devices, vendor decoders, calls/Bluetooth, network changes and sustained loads still require device validation. Windows ZIP updates still need end-to-end validation on Windows. Remote audio and pre-login services remain experimental.

## 日本語

- Windows x64／ARM64 は Portable ZIP で提供します。すべて展開して YourDesk.exe を実行してください。
- Portable ZIP のダウンロード・検証・バックアップ・展開・置換・再起動を自動化し、失敗時は復元を試みます。サービス更新専用パッケージは維持します。
- 正式署名済み Android arm64 の ZIP を同時提供します。展開して APK をインストールしてください。タッチ・音声・複数画面の機能を維持します。

NSIS インストーラーは一時的に提供しません。ZIP 内の Windows EXE は信頼されたコード署名が未対応で、警告が表示される可能性があります。新版は ZIP 自動更新とサービス引き継ぎに対応します。旧アプリ・旧サービスの初回更新は手動操作が必要な場合があります。service ZIP は一般向けインストーラーではありません。旧版が Portable の名前を認識しない場合は本ページから手動取得してください。macOS は署名・公証済み DMG、Linux は CLI Host ZIP、WinPE は実験的機能です。

Android は Android 8.0 以上の arm64 向けリモート Viewer です。ZIP を展開して APK をインストールしてください。正式署名鍵を維持し versionCode を増やすため、同じ鍵の正式版は上書き更新できます。旧 0.1.0 は別署名のため[移行手順](https://github.com/VaderChen/YourDesk/blob/main/docs/ANDROID-RELEASE-2026-10-01.md)が必要です。保存データを失うアンインストールは避けてください。クリップボード同期、専用ファイル転送、AV1、スマートフォンの被操作モードは未対応です。

今回はパッケージとバージョン統一の発行で、Android の機能追加はありません。パッケージ・更新 Smoke、Android フロントエンドとポリシー、Release lint、APK バージョン・正式 v2/v3 署名・16 KB ELF/ZIP 静的検査を対象とします。以前の Android 11 実機結果は旧版のもので、新 APK ですべて再実行したものではありません。Android 16/16 KB 端末、各社デコーダー、通話・Bluetooth、ネットワーク切替、長時間負荷は実機検証が必要です。Windows ZIP 更新は Windows 実機での一貫した検証が必要です。音声とログイン前サービスは実験的機能です。

## 한국어

- Windows x64／ARM64는 Portable ZIP으로 제공합니다. 전체 파일을 압축 해제하고 YourDesk.exe를 실행하세요.
- Portable ZIP 업데이트는 자동 다운로드, 검증, 백업, 압축 해제, 교체 및 재시작을 수행하고 실패 시 복원을 시도합니다. 서비스 전용 패키지는 유지합니다.
- 정식 서명된 Android arm64 ZIP도 제공합니다. 압축 해제 후 APK를 설치하세요. 기존 터치, 오디오, 다중 화면 기능은 유지됩니다.

NSIS 설치 프로그램은 당분간 제공하지 않습니다. ZIP 내부 Windows EXE에는 아직 신뢰할 수 있는 코드 서명이 없어 보안 경고가 나타날 수 있습니다. 신규 버전은 ZIP 자동 업데이트 및 서비스 인계를 지원합니다. 구형 앱/서비스의 최초 업그레이드는 수동 작업이 필요할 수 있습니다. service ZIP은 일반 설치용이 아닙니다. 구버전이 Portable 파일명을 인식하지 못하면 이 페이지에서 직접 다운로드하세요. macOS는 서명 및 공증된 DMG, Linux는 CLI Host ZIP이며 WinPE는 실험적입니다.

Android 8.0 이상 arm64에서 원격 컴퓨터 Viewer로 사용합니다. ZIP을 압축 해제한 후 APK를 설치하세요. 기존 정식 서명 키를 유지하고 versionCode를 올리므로 동일 서명의 정식 버전은 덮어쓰기 업데이트가 가능합니다. 다른 서명의 구형 0.1.0은 [이전 안내](https://github.com/VaderChen/YourDesk/blob/main/docs/ANDROID-RELEASE-2026-10-01.md)를 따르세요. 데이터가 지워지는 삭제 후 재설치는 피하세요. 클립보드 동기화, 독립 파일 전송, AV1, 휴대폰 호스트는 미지원입니다.

이번 릴리스는 패키지 정책과 버전 통일이며 Android 신규 기능은 없습니다. 패키지/업데이트 Smoke, Android 프런트엔드와 정책 검사, Release lint, APK 버전/정식 v2/v3 서명 및 16 KB ELF/ZIP 정적 검사를 검증합니다. 이전 Android 11 실기 결과는 구버전 결과이며 새 APK에서 모든 장치 테스트를 다시 실행한 것은 아닙니다. Android 16/16 KB 장치, 제조사별 디코더, 통화/Bluetooth, 네트워크 변경과 장시간 부하는 실기 검증이 필요합니다. Windows ZIP 업데이트는 Windows 실기의 전체 검증이 필요합니다. 원격 오디오와 로그인 전 서비스는 실험적입니다.
