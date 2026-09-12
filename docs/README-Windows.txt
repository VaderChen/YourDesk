繁體中文
--------------------
登入前啟動（實驗性功能）：在進階設定開啟後，經管理員授權安裝 Windows 自動啟動服務；開機後不需先登入，即啟動 Host，登入後持續運作。關閉時亦需管理員授權。更新或解除安裝前請先停用；不支援 BitLocker 開機前解鎖，登入畫面操作仍待實機驗證。

執行 setup.exe 安裝後，從桌面或開始功能表啟動 YourDesk。需要 Microsoft Edge WebView2 Runtime。
新增站台並輸入對方 ID，選擇桌面或命令列圖示，再輸入對方密碼。雙擊站台可選擇模式。
設定 → 網路安全可修改本機連線密碼。關閉視窗後仍常駐；從 Tray 選擇關閉程式才會結束。

CLI：-signal 指定配對伺服器；-print-uid 顯示裝置 ID；-print-secret 顯示或首次建立已儲存的密碼後結束。
設定本次連線密碼：使用 -secret "你的密碼"，可直接啟動，不需輸入 JSON。密碼至少 6 個字元、最多 1000 bytes；只套用本次啟動，不修改已儲存密碼。省略時使用已儲存密碼，首次自動產生。-secret-stdin 保留給自動化使用，不可與 -secret 同時指定。

.\yourdesk-client.exe -signal wss://desktop.mars-cloud.com:8080/ws -secret "ReplaceWithYourPassword"

.\yourdesk-client.exe -print-uid
.\yourdesk-client.exe -print-secret

English
--------------------
Start before login (experimental): enable it in Advanced Settings to install an automatic Windows service with administrator approval. The Host starts at boot before sign-in and continues after sign-in. Disabling also requires approval. Turn it off before updating or uninstalling. BitLocker pre-boot unlock is unsupported; login-screen control still requires on-device validation.

Run setup.exe and launch YourDesk from the desktop or Start menu. Microsoft Edge WebView2 Runtime is required.
Add a site with the remote ID, choose Desktop or Terminal, then enter the remote password. Double-click a site to choose a mode.
Change the local connection password in Settings → Network Security. Closing the window keeps the app running; choose Quit in the tray menu to exit.

CLI: -signal selects the pairing server; -print-uid shows the device ID; -print-secret displays or initially creates the saved password and exits.
Set the password for this run with -secret "your password"; no JSON input is needed. Use at least 6 characters and at most 1000 bytes. The saved password is unchanged. If omitted, the saved password is used or generated on first use. -secret-stdin remains available for automation and cannot be combined with -secret.

.\yourdesk-client.exe -signal wss://desktop.mars-cloud.com:8080/ws -secret "ReplaceWithYourPassword"

.\yourdesk-client.exe -print-uid
.\yourdesk-client.exe -print-secret

日本語
--------------------
ログイン前の起動（実験的機能）：詳細設定から管理者の許可で Windows 自動起動サービスをインストールします。ログイン前に Host が起動し、ログイン後も継続します。無効化にも管理者の許可が必要です。更新やアンインストール前に無効にしてください。BitLocker の起動前解除には非対応で、ログイン画面の操作は実機確認が必要です。

setup.exe でインストールし、デスクトップまたはスタートメニューから起動します。Microsoft Edge WebView2 Runtime が必要です。
接続先の ID を登録し、デスクトップまたはターミナルを選んで相手のパスワードを入力します。ダブルクリックでもモードを選べます。
本機の接続パスワードは設定 → ネットワークセキュリティで変更します。ウィンドウを閉じても常駐するため、終了はトレイメニューから行います。

CLI：-signal は配対サーバー、-print-uid は装置 ID 表示、-print-secret は保存済みパスワードの表示（初回は生成）後に終了します。
今回のパスワードは -secret "パスワード" で指定します。JSON 入力は不要です。6 文字以上、1000 bytes 以下で、保存済みパスワードは変更しません。省略時は保存済みの値を使用し、初回は自動生成します。自動化用の -secret-stdin とは併用できません。

.\yourdesk-client.exe -signal wss://desktop.mars-cloud.com:8080/ws -secret "ReplaceWithYourPassword"

.\yourdesk-client.exe -print-uid
.\yourdesk-client.exe -print-secret

한국어
--------------------
로그인 전 시작(실험적 기능): 고급 설정에서 관리자 승인으로 Windows 자동 시작 서비스를 설치합니다. 부팅 시 로그인 전에 Host가 시작되며 로그인 후에도 계속 실행됩니다. 끌 때도 승인이 필요합니다. 업데이트 또는 제거 전에 꺼 주세요. BitLocker 부팅 전 잠금 해제는 지원하지 않으며 로그인 화면 제어는 실제 장치에서 확인이 필요합니다.

setup.exe로 설치한 후 바탕 화면이나 시작 메뉴에서 실행하세요. Microsoft Edge WebView2 Runtime이 필요합니다.
상대 장치 ID로 연결을 추가하고 데스크톱 또는 터미널을 선택한 뒤 상대 암호를 입력하세요. 더블 클릭으로도 모드를 선택할 수 있습니다.
로컬 연결 암호는 설정 → 네트워크 보안에서 변경합니다. 창을 닫아도 실행되므로 완전히 종료하려면 트레이 메뉴를 사용하세요.

CLI: -signal은 연결 서버를 지정하고, -print-uid는 장치 ID를 표시합니다. -print-secret은 저장된 암호를 표시하거나 처음 생성한 뒤 종료합니다.
이번 실행의 암호는 -secret "암호"로 지정합니다. JSON 입력은 필요 없습니다. 최소 6자, 최대 1000 bytes이며 저장된 암호는 변경하지 않습니다. 생략하면 저장된 암호를 사용하고 최초 실행 시 자동 생성합니다. 자동화용 -secret-stdin과 함께 사용할 수 없습니다.

.\yourdesk-client.exe -signal wss://desktop.mars-cloud.com:8080/ws -secret "ReplaceWithYourPassword"

.\yourdesk-client.exe -print-uid
.\yourdesk-client.exe -print-secret
