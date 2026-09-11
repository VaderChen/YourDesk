## 繁體中文

- 本機被遠端連線時，Tray 新增「關閉遠端連線」，設定按鈕旁也會顯示紅色停止圖示；停止目前連線後仍可接受後續連線。
- MCP 白名單啟用時，名單內免 Token、名單外拒絕；關閉白名單時，所有來源仍需 Bearer Token。預設僅允許 `127.0.0.1`。
- 改善 Mac 對 Mac 的 Caps Lock 輸入法切換，以及遠端重新啟動後的連線狀態處理。
- 自動配置改用獨立進度視窗顯示偵測步驟，調整資料取樣；缺少解碼 FPS 時顯示 `-`，關於頁加入 GitHub 連結。
- 加入 macOS「未登入開機」實驗實作，預設關閉；需要 macOS 14.4 以上、正式簽署的 App、管理員授權及既有螢幕錄製／輔助使用權限。Windows 暫不支援，開關停用。

**實驗功能限制：未登入連線尚未完成實機登入／登出驗證，不支援 FileVault 開機前解鎖。啟用後須先停用服務，才能修改連線設定或自動安裝更新。遠端開機仍未實作。**

## English

- Added “Disconnect incoming connection” to the tray and a red stop icon beside Settings while this computer is being controlled. Future connections remain available.
- With the MCP allowlist enabled, listed IPs require no token and other IPs are blocked. With it disabled, all sources require a Bearer token. The default list contains only `127.0.0.1`.
- Improved Mac-to-Mac Caps Lock input-source switching and connection-state handling after remote restarts.
- Automatic configuration now shows detection steps in a separate progress dialog; sampling was adjusted, unavailable decoding FPS displays `-`, and About includes a GitHub link.
- Added an experimental macOS pre-login service, off by default, requiring macOS 14.4+, a signed app, administrator authorization and existing screen-recording/accessibility permissions. Unsupported on Windows; its switch remains disabled.

**Experimental limitations: login/logout operation has not been verified on hardware, and FileVault pre-boot unlock is unsupported. Disable the service before changing connection settings or automatically installing updates. Remote power-on remains unimplemented.**

## 日本語

- このパソコンへの接続中、トレイに接続終了メニュー、設定の横に赤い停止アイコンを表示します。切断後も新しい接続を受け付けます。
- MCP 許可リスト有効時はリスト内の IP は Token 不要、リスト外は拒否します。無効時はすべて Bearer Token が必要です。初期リストは `127.0.0.1` のみです。
- Mac 間の Caps Lock による入力ソース切り替えと、接続先再起動後の接続状態処理を改善しました。
- 自動設定では別の進捗ダイアログに検出手順を表示し、サンプリングを調整しました。取得できないデコード FPS は `-` と表示し、バージョン情報に GitHub リンクを追加しました。
- macOS のログイン前接続サービスを実験的に追加しました。初期状態は無効で、macOS 14.4 以降、署名済み App、管理者認証、画面収録・アクセシビリティ権限が必要です。Windows は未対応でスイッチは無効です。

**実験機能の制限：実機でのログイン・ログアウト動作は未検証で、FileVault の起動前解除には対応しません。接続設定の変更や更新の自動インストール前にサービスを無効にしてください。遠隔起動は未実装です。**

## 한국어

- 이 컴퓨터가 원격 연결된 동안 트레이에 연결 종료 메뉴와 설정 옆에 빨간 정지 아이콘을 표시합니다. 종료 후에도 새 연결을 받을 수 있습니다.
- MCP 허용 목록이 켜져 있으면 목록 내 IP는 Token 없이 연결되고 나머지는 차단됩니다. 끄면 모든 연결에 Bearer Token이 필요합니다. 기본 목록은 `127.0.0.1`뿐입니다.
- Mac 간 Caps Lock 입력 소스 전환과 원격 컴퓨터 재시작 후 연결 상태 처리를 개선했습니다.
- 자동 설정 시 별도 진행 창에 감지 단계를 표시하고 표본 수집을 조정했습니다. 디코딩 FPS가 없으면 `-`로 표시하며 정보 화면에 GitHub 링크를 추가했습니다.
- macOS 로그인 전 연결 서비스를 실험적으로 추가했습니다. 기본으로 꺼져 있으며 macOS 14.4 이상, 서명된 App, 관리자 승인 및 화면 기록·손쉬운 사용 권한이 필요합니다. Windows는 미지원으로 스위치가 비활성화됩니다.

**실험 기능 제한: 실제 기기의 로그인·로그아웃 동작은 아직 검증하지 않았으며 FileVault 부팅 전 잠금 해제는 지원하지 않습니다. 연결 설정 변경이나 자동 업데이트 설치 전에 서비스를 꺼야 합니다. 원격 전원 켜기는 미구현입니다.**

## 套件與驗證 / Packages and validation

提供 macOS Apple Silicon DMG 與 Windows x64／ARM64 Installer。本次執行平台編譯、語法檢查與 macOS 簽章／Apple 公證，不新增實機功能測試。

Includes a macOS Apple Silicon DMG and Windows x64/ARM64 installers. Validation covers platform builds, syntax checks and macOS signing/Apple notarization; no additional hardware functionality tests were run.
