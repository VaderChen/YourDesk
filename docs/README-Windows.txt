繁體中文
--------
執行 YourDesk-版本-windows-架構-setup.exe，依精靈完成安裝，再從桌面或開始功能表啟動 YourDesk。
安裝在目前使用者的 LocalAppData\Programs\YourDesk，不需管理員權限；可從 Windows 應用程式清單解除安裝，設定及下載資料會保留。
電腦需要 Microsoft Edge WebView2 Runtime。關閉視窗後仍常駐 Tray，左鍵開啟主畫面、右鍵彈出選單；選擇「關閉程式」才完整結束。
下載更新完成後以紅字倒數 10 秒，可取消或立即更新；倒數結束後自動關閉、安裝與重啟。遠端連線會中斷。
舊版 ZIP 需手動解壓安裝；僅支援 ZIP 的舊更新器，第一次改用 Installer 時需手動下載安裝程式。
剪貼簿支援文字（1 MiB）、圖片（32 MiB）、檔案與目錄（每批 2 GiB，目錄含封存資訊），最多 64 個最外層項目；目錄最多 4096 筆、64 層，保留空目錄。雙方需更新，舊版只提供既有能力，文字備援上限 32 KiB。不支援符號連結與特殊檔案。傳輸在背景執行，不顯示進度 DLG。

English
-------
Run YourDesk-version-windows-architecture-setup.exe, then launch YourDesk from the desktop or Start menu.
Installation is per user in LocalAppData\Programs\YourDesk. No administrator rights are required. Uninstall from Windows Apps; settings and downloaded data are retained.
Microsoft Edge WebView2 Runtime is required. Left-click the tray icon to open the app; right-click for the menu. Choose Quit to fully exit.
After downloading and verification, a red 10-second countdown allows cancellation or immediate updating. The app then quits, installs and restarts automatically.
Legacy ZIP packages remain supported. Older ZIP-only updaters require a manual installer download for the first migration.
Clipboard: text 1 MiB, images 32 MiB, files and folders 2 GiB per batch including archive metadata. Up to 64 top-level items; folders allow 4096 entries and 64 levels, including empty folders. Both ends must support folders. Legacy text fallback is 32 KiB. Symbolic links and special files are unsupported. Transfers run in the background without a progress dialog.

日本語
------
YourDesk-version-windows-architecture-setup.exe を実行し、デスクトップまたはスタートメニューから起動します。
現在のユーザーの LocalAppData\Programs\YourDesk にインストールされ、管理者権限は不要です。Windows のアプリ一覧から削除できます。設定とダウンロード済みデータは保持されます。
Microsoft Edge WebView2 Runtime が必要です。トレイの左クリックで画面を開き、右クリックでメニューを表示します。終了はトレイメニューから行います。
更新ダウンロード後は赤字で 10 秒カウントダウンします。キャンセルまたは即時更新を選べます。終了後は自動終了、インストール、再起動を行います。
従来の ZIP にも対応します。ZIP 専用の旧更新機能から移行する場合、最初のインストーラーは手動でダウンロードしてください。
クリップボードは文字 1 MiB、画像 32 MiB、ファイルとフォルダーは管理情報を含め 1 回 2 GiB まで。最上位 64 項目、フォルダーは 4096 項目・64 階層までで空フォルダーも保持します。両端の更新が必要です。旧方式の文字転送は 32 KiB まで。リンクと特殊ファイルは非対応です。進捗ダイアログは表示しません。

한국어
------
YourDesk-version-windows-architecture-setup.exe를 실행한 후 바탕 화면 또는 시작 메뉴에서 YourDesk를 시작하세요.
현재 사용자의 LocalAppData\Programs\YourDesk에 설치되며 관리자 권한이 필요하지 않습니다. Windows 앱 목록에서 제거할 수 있으며 설정과 다운로드한 데이터는 유지됩니다.
Microsoft Edge WebView2 Runtime이 필요합니다. 트레이 왼쪽 클릭은 메인 화면을 열고 오른쪽 클릭은 메뉴를 표시합니다. 완전히 종료하려면 트레이 메뉴를 사용하세요.
업데이트 다운로드 후 빨간색으로 10초를 카운트다운하며 취소하거나 즉시 업데이트할 수 있습니다. 이후 자동 종료, 설치 및 재시작합니다.
이전 ZIP 패키지도 지원합니다. ZIP만 인식하는 이전 업데이트 기능에서는 최초 설치 프로그램을 수동으로 다운로드해야 합니다.
클립보드는 텍스트 1 MiB, 이미지 32 MiB, 파일 및 폴더는 메타데이터를 포함하여 한 번에 2 GiB까지 지원합니다. 최상위 64개, 폴더 내부 4096개 항목과 64단계까지 가능하며 빈 폴더를 유지합니다. 양쪽 업데이트가 필요합니다. 이전 텍스트 전송은 32 KiB까지 지원합니다. 심볼릭 링크와 특수 파일은 지원하지 않으며 진행 대화상자는 표시하지 않습니다.

檔案與目錄改為先建立快照、公布清單，系統貼上或讀取內容時才傳輸；右鍵貼上亦適用。新流程需兩端更新。取消固定限速的調整只需更新來源端即可改善該方向。安裝新版前請從 Tray 結束舊版與 遠端顯示。


MCP / 本版更新 / Changes / 更新内容 / 변경 사항
新增 MCP：AI Agent 可連到遠端、取得畫面、操作鍵盤滑鼠、斷線、診斷及調整設定。設定 → MCP 設定中啟用，預設關閉；白名單預設開啟且僅允許 127.0.0.1，名單內免 Token、名單外拒絕；關閉白名單時所有來源須 Token。畫面預設隱藏，Tray 提供操作提示與開啟／隱藏選項。
剪貼簿同步改善：Mac 對 Mac 的文字、圖片及雙向檔案複製已由使用者實機確認可用。

New MCP support lets AI agents connect remotely, capture screenshots, control keyboard/mouse, disconnect, run diagnostics and manage settings. Enable it in MCP Settings; it defaults to off. The allowlist defaults to on with only 127.0.0.1, listed IPs need no token and other IPs are blocked while enabled; when disabled, all sources require a token. Windows are hidden by default, with tray indicators and show/hide controls.
Clipboard synchronization improved: the user confirmed that Mac-to-Mac text, image and bidirectional file copy work on real devices.

新しい MCP 機能により、AI Agent からリモート接続、画面取得、キーボード・マウス操作、切断、診断、設定変更ができます。MCP 設定で有効にしてください。初期状態は無効、IP 許可リストは有効で 127.0.0.1 のみ許可します。リスト内は Token 不要、リスト外は拒否し、リスト無効時はすべて Token が必要です。画面は初期状態で非表示となり、トレイで通知と表示切替ができます。
クリップボード同期を改善しました。Mac 間のテキスト、画像、双方向ファイルコピーが実機で利用できることをユーザーが確認しました。

새 MCP 기능으로 AI Agent가 원격 연결, 화면 캡처, 키보드·마우스 제어, 연결 해제, 진단 및 설정 변경을 할 수 있습니다. MCP 설정에서 활성화하세요. 기본값은 꺼짐이며 IP 허용 목록은 켜짐으로 127.0.0.1만 허용합니다. 목록 내 IP는 Token이 필요 없고 나머지는 차단되며, 목록을 끄면 모든 연결에 Token이 필요합니다. 화면은 기본적으로 숨겨지며 트레이에서 알림과 표시 전환을 제공합니다.
클립보드 동기화 개선: 사용자가 실제 기기에서 Mac 간 텍스트, 이미지 및 양방향 파일 복사가 가능함을 확인했습니다.

MCP: http://127.0.0.1:12345/mcp
https://github.com/VaderChen/YourDesk/blob/main/docs/MCP.md


## Windows 裝置 ID 更新與重複 Host 修正
已修正 Windows 因裝置 ID 撞號而出現的重複 Host 占用問題。 舊版使用 CPU ID（ProcessorId），不同電腦可能取得相同值；新版改用 Windows MachineGuid，無效時改用 SMBIOS UUID。

Windows 更新後的裝置 ID（序號）會改變。請在更新後查看新 ID，並修改其他電腦已儲存的連線站台；不要繼續使用舊 ID。Mac ID 不受影響。

新增／編輯站台與快速連線可只輸入 ID 的英數字，程式自動轉大寫、補上 `YD-` 與每四碼的 `-`；也支援貼上完整 ID 及輸入 IP 位址。

## Windows device ID update and duplicate Host fix
Fixed duplicate Host occupancy caused by colliding Windows device IDs. Older versions used CPU ProcessorId values, which can be identical on different computers. Windows now uses MachineGuid, with SMBIOS UUID as a fallback.

Your Windows device ID will change after updating. Check the new ID and update saved connections on your other computers; stop using the old ID. Mac IDs are unchanged.

When adding/editing a connection or using Quick Connect, enter only the ID's letters and numbers; uppercase, `YD-` and four-character separators are added automatically. Full IDs and IP addresses can still be entered.

## Windows 装置 ID の更新と Host 重複問題の修正
Windows の装置 ID 重複による Host 占有問題を修正しました。 旧版で使用していた CPU の ProcessorId は、別のパソコンでも同じ値になる場合があります。新版は Windows MachineGuid を使用し、無効な場合は SMBIOS UUID を使用します。

更新後、Windows の装置 ID（識別番号）が変わります。新しい ID を確認し、他のパソコンに保存した接続先を更新してください。古い ID は使用しないでください。Mac の ID は変わりません。

接続先の追加・編集とクイック接続では、ID の英数字だけを入力すると、大文字化、`YD-`、4 文字ごとの `-` を自動で適用します。完全な ID の貼り付けや IP アドレスの入力にも対応します。

## Windows 장치 ID 변경 및 중복 Host 수정
Windows 장치 ID 충돌로 발생하던 중복 Host 점유 문제를 수정했습니다. 이전 버전의 CPU ProcessorId는 서로 다른 컴퓨터에서 같을 수 있습니다. 이제 Windows MachineGuid를 사용하고 유효하지 않으면 SMBIOS UUID를 사용합니다.

업데이트 후 Windows 장치 ID가 변경됩니다. 새 ID를 확인하고 다른 컴퓨터에 저장한 연결 정보를 수정하세요. 이전 ID는 사용하지 마세요. Mac ID는 변경되지 않습니다.

연결 추가·편집과 빠른 연결에서 ID의 영문자와 숫자만 입력하면 대문자, `YD-` 및 4자리마다 `-`가 자동 적용됩니다. 전체 ID 붙여넣기와 IP 주소 입력도 지원합니다.
