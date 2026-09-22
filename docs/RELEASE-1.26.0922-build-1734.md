## 繁體中文

- 新增獨立檔案傳輸視窗：遠端 Windows 從 `C:\`、Mac 從 `~/` 開始，可返回上層並瀏覽使用者有權限存取的磁碟。資料夾優先、各組依名稱排序，隱藏項目不顯示，並以 Font Awesome 圖示區分資料夾與文件。
- 修正本機選檔視窗，支援複選上傳；遠端可勾選、Cmd／Ctrl 加選、Shift 範圍選取及批次下載／刪除。下載改為直接選擇本機目的資料夾，保留暫停、手動續傳與進度控制，移除拖出下載區塊。
- 新增預設關閉的實驗性遠端聲音。預設 Opus，品質跟隨影像的低流量／標準／高畫質；自動選擇依 AAC 硬體 → Opus → AAC 軟體 → PCM，比對兩端可用能力。AAC 編解碼在實測支援時優先硬體加速，工具列分別顯示影像／聲音的編碼與解碼狀態。
- 登入前服務開啟時，APP 內自動更新會交接服務、更新後恢復服務與 APP。Windows 首次安裝或舊服務遷移需一次管理員授權，後續支援的服務更新沿用已授權服務；macOS 更新服務仍需管理員授權。
- APP 啟動不再詢問 Ctrl+Alt+Del，改由進階設定開關手動授權；縮小設定字體與開關，修正虛擬鍵盤右 Alt／AltGr 及編碼提示的間距。

兩端請一併更新，以使用完整檔案導覽與自動聲音協商。批次下載限一般檔案，每批最多 64 個、合計 2 GiB；不覆寫同名檔案。刪除是永久操作，批次遇未確認結果即停止。續傳需保持兩端 APP 及傳輸視窗開啟。

已完成相關 Go 測試、51 項前端回歸、四語系瀏覽器 Smoke、Mac 原生選檔／檔案視窗 Smoke，以及 Mac／Windows 建置。Windows 原生選檔、系統隱藏屬性、連續服務更新與跨平台音訊仍需實機驗證；登入前服務、遠端聲音與 WinPE 維持實驗性。手動安裝／Windows 免安裝版的服務交接仍需手動處理。服務更新 ZIP 僅供 APP 自動更新使用。

## English

- Add a dedicated file-transfer window. Remote Windows starts at `C:\` and Mac at `~/`, with parent navigation and access to disks allowed by the user's permissions. Folders come first, names are sorted within each group, hidden items are omitted, and Font Awesome icons distinguish folders from files.
- Fix the local file picker and support multiple uploads. Remote selection supports checkboxes, Cmd/Ctrl, Shift ranges, and batch downloads/deletion. Choose a local destination folder once for downloads; pause, manual resume and progress controls remain available. Remove the download drag-out area.
- Add experimental remote audio, off by default. Opus is the default codec and follows the existing video quality menu. Automatic selection checks both ends in this order: AAC hardware → Opus → AAC software → PCM. AAC uses hardware encoding/decoding when verified available. The toolbar shows separate video/audio encoder and decoder status.
- In-app updates now hand off an enabled pre-login service and restore the service and app afterward. Windows needs one administrator approval for initial installation or legacy-service migration; subsequent supported updates use the authorized service. macOS service updates still require administrator approval.
- Remove the Ctrl+Alt+Del startup prompt; authorization is controlled manually by an Advanced Settings switch. Reduce settings text/switch sizes and fix virtual-keyboard right Alt/AltGr and codec-tooltip spacing.

Update both computers for full file navigation and automatic audio negotiation. Batch downloads support regular files only, up to 64 files and 2 GiB per batch, without overwriting existing names. Deletion is permanent and stops on an uncertain result. Resume requires both apps and the transfer window to remain open.

Relevant Go tests, 51 frontend regressions, four-language browser smoke tests, native Mac file-picker/file-window smoke tests, and Mac/Windows builds passed. Native Windows file selection, hidden attributes, consecutive service updates and cross-platform audio still need on-device validation. Pre-login services, remote audio and WinPE remain experimental. Manual installations and Windows portable updates still require manual service handoff. Service-update ZIPs are for the app updater only.

## 日本語

- 独立したファイル転送ウィンドウを追加しました。リモート Windows は `C:\`、Mac は `~/` から開始し、上位フォルダーやユーザー権限でアクセス可能なディスクを参照できます。フォルダーを先に、各グループを名前順に表示し、隠し項目を除外します。Font Awesome のアイコンで種類を区別します。
- ローカルのファイル選択を修正し、複数ファイルのアップロードに対応しました。リモートではチェックボックス、Cmd／Ctrl、Shift による複数選択と一括ダウンロード／削除が可能です。保存先は一度選択するだけで、一時停止・手動再開・進捗表示を利用できます。ダウンロードのドラッグ領域を削除しました。
- 初期設定でオフの実験的なリモート音声を追加しました。標準コーデックは Opus で、品質は映像のメニューに連動します。自動選択は両端の対応状況を確認し、AAC ハードウェア → Opus → AAC ソフトウェア → PCM の順に選びます。AAC は利用可能と検証されたハードウェアを優先し、ツールバーで映像／音声のエンコード・デコード状態を個別表示します。
- ログイン前サービスが有効な場合、アプリ内更新でサービスを引き継ぎ、更新後にサービスとアプリを復元します。Windows の初回導入または旧サービスの移行には一度管理者承認が必要ですが、以後の対応更新は承認済みサービスを使用します。macOS のサービス更新には引き続き管理者承認が必要です。
- 起動時の Ctrl+Alt+Del 承認を廃止し、詳細設定のスイッチで手動操作するようにしました。設定の文字・スイッチを小さくし、仮想キーボードの右 Alt／AltGr とコーデック表示の間隔を修正しました。

完全なファイル参照と音声の自動選択には両端を更新してください。一括ダウンロードは通常ファイルのみ、最大 64 個・合計 2 GiB で、同名ファイルを上書きしません。削除は完全削除で、結果が不明な場合は処理を停止します。再開には両端のアプリと転送ウィンドウを開いたままにする必要があります。

関連 Go テスト、51 項目のフロントエンド回帰、4 言語のブラウザー Smoke、Mac のネイティブ選択／転送ウィンドウ Smoke、Mac／Windows のビルドを確認しました。Windows の実機ファイル選択・隠し属性・連続サービス更新とクロスプラットフォーム音声は実機未検証です。ログイン前サービス、音声、WinPE は実験的機能です。手動インストールと Windows ポータブル版ではサービスの手動引き継ぎが必要です。サービス更新 ZIP はアプリの更新専用です。

## 한국어

- 전용 파일 전송 창을 추가했습니다. 원격 Windows는 `C:\`, Mac은 `~/`에서 시작하며 상위 폴더와 사용자 권한으로 접근 가능한 디스크를 탐색할 수 있습니다. 폴더를 먼저 표시하고 각 그룹을 이름순으로 정렬하며 숨김 항목은 표시하지 않습니다. Font Awesome 아이콘으로 폴더와 파일을 구분합니다.
- 로컬 파일 선택 창을 수정하고 여러 파일 업로드를 지원합니다. 원격 목록에서 체크박스, Cmd/Ctrl, Shift 범위 선택과 일괄 다운로드/삭제를 사용할 수 있습니다. 다운로드 대상 폴더는 한 번 선택하며 일시 정지, 수동 재개 및 진행률을 유지합니다. 다운로드 끌어내기 영역을 제거했습니다.
- 기본으로 꺼진 실험적 원격 오디오를 추가했습니다. 기본 코덱은 Opus이며 기존 영상 품질 메뉴를 따릅니다. 자동 선택은 양쪽 지원 여부를 확인해 AAC 하드웨어 → Opus → AAC 소프트웨어 → PCM 순서로 선택합니다. AAC는 사용 가능성이 검증된 하드웨어를 우선하며 도구 모음에서 영상/음성 인코딩과 디코딩 상태를 따로 표시합니다.
- 로그인 전 서비스가 켜져 있으면 앱 내 업데이트가 서비스를 인계하고 완료 후 서비스와 앱을 복원합니다. Windows 최초 설치 또는 구형 서비스 이전에는 한 번의 관리자 승인이 필요하며 이후 지원되는 업데이트는 승인된 서비스를 사용합니다. macOS 서비스 업데이트에는 계속 관리자 승인이 필요합니다.
- 앱 시작 시 Ctrl+Alt+Del 승인 요청을 없애고 고급 설정 스위치에서 수동으로 관리하도록 했습니다. 설정 글꼴과 스위치를 줄이고 가상 키보드의 오른쪽 Alt/AltGr 및 코덱 도움말 간격을 수정했습니다.

전체 파일 탐색과 자동 오디오 협상을 사용하려면 양쪽을 업데이트하세요. 일괄 다운로드는 일반 파일만 지원하며 한 번에 최대 64개, 합계 2 GiB이고 같은 이름을 덮어쓰지 않습니다. 삭제는 영구적이며 결과가 불확실하면 중단됩니다. 재개하려면 양쪽 앱과 전송 창을 열어 두어야 합니다.

관련 Go 테스트, 프런트엔드 회귀 51개, 4개 언어 브라우저 Smoke, Mac 네이티브 파일 선택/전송 창 Smoke 및 Mac/Windows 빌드를 통과했습니다. Windows의 실제 파일 선택, 숨김 속성, 연속 서비스 업데이트와 플랫폼 간 오디오는 실기 검증이 필요합니다. 로그인 전 서비스, 오디오 및 WinPE는 실험적 기능입니다. 수동 설치와 Windows 포터블 업데이트는 서비스 인계를 수동으로 처리해야 합니다. 서비스 업데이트 ZIP은 앱 업데이트 전용입니다.
