## 繁體中文

- 修正近端為 Mac 時，複製遠端檔案後 Finder「貼上」反灰或沒有反應。接收端先確認可存取貼上所需的網路卷宗，再發布檔案清單；使用者已實測確認恢復可用。
- 這項修正只需更新近端 Mac。若系統詢問「網路卷宗」權限，請允許後重新複製。曾拒絕時，可到「系統設定 → 隱私權與安全性 → 檔案與檔案夾」允許 YourDesk 存取網路卷宗。
- 授權遭拒時保留原剪貼簿並提示處理方式，同一份清單不反覆建立掛載。檔案內容仍在實際讀取時傳輸。
- 「封包分析」啟用後，新增剪貼簿清單接收、掛載、存取檢查及發布結果紀錄；只記錄階段與數量，不記錄剪貼簿內容、檔名或路徑。

驗證：本次修正在 build 1509 已獲使用者實機確認，正式套件統一為 build 1524。語法／編譯、雙程序自動同步與拒絕存取 Smoke 均通過；雙向各 3.29 MB 內容比對一致。原失效回報涵蓋遠端 macOS 與 Windows，但此次有效回覆未逐項指定重測組合，不將其擴大為所有平台與檔案情境均已驗證。

套件：macOS arm64 簽章／公證 DMG、Windows x64／arm64 安裝版、Windows x64 免安裝 ZIP、Linux x64／arm64 CLI Host、WinPE x64 實驗版。Android 不納入本次發行。沿用既有串流穩定修正；先前串流停滯回報僅見於 macOS 對 macOS，仍不宣稱所有斷線均已完全解決，背景自動重連／續傳維持未啟用。

## English

- Fixes Finder's disabled or unresponsive Paste action when receiving remote files on a Mac. The receiver checks access to the required network volume before publishing the file list. The user confirmed that pasting works again.
- Only the receiving Mac needs this fix. Allow Network Volumes access if prompted, then copy again. If previously denied, enable it for YourDesk under System Settings → Privacy & Security → Files and Folders.
- Denied access preserves the existing clipboard and provides recovery guidance. Retries of the same offer do not repeatedly mount a volume. File contents are still transferred on demand.
- When Packet Analysis is enabled, clipboard diagnostics record list reception, mounting, access checks and publication results, without clipboard contents, filenames or paths.

Validation: the user confirmed the fix in build 1509; release packages use build 1524. Syntax/compilation, two-process automatic synchronization and denied-access smoke checks passed, with matching 3.29 MB transfers in each direction. Original failures involved both macOS and Windows senders, but the successful retest did not identify each combination; this is not validation of every platform and file scenario.

Packages: signed/notarized macOS arm64 DMG; Windows x64/arm64 installers; Windows x64 portable ZIP; Linux x64/arm64 CLI Host; experimental WinPE x64. No Android release. Prior streaming fixes remain; earlier freezes were reported only between Macs, and not all disconnects are confirmed resolved. Background auto-reconnect and transfer resumption remain disabled.

## 日本語

- 手元の Mac でリモートファイルを受信した際、Finder の「ペースト」が無効または無反応になる問題を修正しました。ファイル一覧を公開する前に必要なネットワークボリュームへのアクセスを確認します。ユーザーが復旧を実機で確認しました。
- この修正は受信側 Mac の更新のみで適用できます。ネットワークボリュームの許可を求められたら許可し、再度コピーしてください。以前拒否した場合は「システム設定 → プライバシーとセキュリティ → ファイルとフォルダ」で YourDesk のアクセスを許可してください。
- アクセス拒否時は元のクリップボードを保持し、対処方法を案内します。同じ一覧の再試行でマウントを繰り返し作成しません。ファイル内容は必要時に転送します。
- パケット解析を有効にすると、一覧受信、マウント、アクセス確認、公開結果を記録します。クリップボード内容、ファイル名、パスは記録しません。

検証：build 1509 の修正をユーザーが実機で確認し、正式パッケージは build 1524 に統一しました。構文／コンパイル、別プロセス間の自動同期、アクセス拒否の Smoke が完了し、双方向各 3.29 MB の内容が一致しました。当初は macOS と Windows の送信元で失敗していましたが、成功報告では再検証した組み合わせが個別に指定されていないため、全環境の検証完了とはしません。

macOS arm64 署名／公証 DMG、Windows x64／arm64 インストーラー、Windows x64 ポータブル、Linux x64／arm64 CLI Host、実験版 WinPE x64 を配布します。Android は対象外です。従来のストリーミング修正を維持します。停止報告は Mac 間のみですが、全切断の解決は断定しません。背景自動再接続・転送再開は無効のままです。

## 한국어

- 로컬 Mac에서 원격 파일을 받을 때 Finder의 붙여넣기가 비활성화되거나 반응하지 않는 문제를 수정했습니다. 파일 목록을 등록하기 전에 필요한 네트워크 볼륨 접근을 확인합니다. 사용자가 실제 기기에서 복구를 확인했습니다.
- 이 수정은 수신 Mac만 업데이트하면 됩니다. 네트워크 볼륨 권한을 요청하면 허용한 후 다시 복사하세요. 이전에 거부했다면 시스템 설정 → 개인정보 보호 및 보안 → 파일 및 폴더에서 YourDesk의 접근을 허용하세요.
- 접근이 거부되면 기존 클립보드를 보존하고 해결 방법을 안내합니다. 같은 목록을 재시도할 때 마운트를 반복 생성하지 않습니다. 파일 내용은 실제 읽기 요청 시 전송됩니다.
- 패킷 분석을 켜면 목록 수신, 마운트, 접근 확인 및 등록 결과를 기록합니다. 클립보드 내용, 파일 이름 및 경로는 기록하지 않습니다.

검증: 사용자가 build 1509의 수정을 실제 기기에서 확인했으며 정식 패키지는 build 1524로 통일했습니다. 구문／컴파일, 별도 프로세스 간 자동 동기화 및 접근 거부 Smoke 검사를 통과했고 양방향 각 3.29 MB의 내용이 일치했습니다. 최초 실패는 macOS와 Windows 송신 측 모두에서 보고되었으나 성공한 재시험의 조합은 개별적으로 명시되지 않았으므로 모든 환경이 검증되었다고 확대하지 않습니다.

서명／공증된 macOS arm64 DMG, Windows x64／arm64 설치 프로그램, Windows x64 포터블 ZIP, Linux x64／arm64 CLI Host, 실험용 WinPE x64를 제공합니다. Android는 제외합니다. 기존 스트리밍 수정을 유지하며 이전 멈춤 보고는 Mac 간에서만 발생했습니다. 모든 연결 끊김이 해결되었다고 단정하지 않으며 백그라운드 자동 재연결／전송 재개는 비활성 상태입니다.
