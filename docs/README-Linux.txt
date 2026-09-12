繁體中文
--------------------
登入後自動啟動（一般使用者、需 systemd 使用者工作階段）：-autostart on 開啟、off 關閉、status 查看。設定後於下次使用者工作階段啟動；不啟用 linger、不取得 root 權限。使用已儲存密碼，可用 -print-secret 查看；設定自動啟動時不要加 -secret 或 stdin 參數。請保留解壓目錄的位置。

./yourdesk-client -autostart on -signal wss://desktop.mars-cloud.com:8080/ws
./yourdesk-client -autostart off
./yourdesk-client -autostart status

解壓符合電腦架構的套件，以一般使用者執行下方 Host 指令。root 不提供互動 Shell。
本版只接受命令列連線；由新版 Mac／Windows YourDesk 新增此裝置 ID，選擇命令列並輸入密碼。沒有 Linux 圖形介面或 REMOTE。按 Ctrl+C 停止 Host。

CLI：-signal 指定配對伺服器；-print-uid 顯示裝置 ID；-print-secret 顯示或首次建立已儲存的密碼後結束。
設定本次連線密碼：使用 -secret "你的密碼"，可直接啟動，不需輸入 JSON。密碼至少 6 個字元、最多 1000 bytes；只套用本次啟動，不修改已儲存密碼。省略時使用已儲存密碼，首次自動產生。-secret-stdin 保留給自動化使用，不可與 -secret 同時指定。

env -u DISPLAY -u WAYLAND_DISPLAY ./yourdesk-client -signal wss://desktop.mars-cloud.com:8080/ws -secret "ReplaceWithYourPassword"

./yourdesk-client -print-uid
./yourdesk-client -print-secret

English
--------------------
Login startup requires a regular account with a working systemd user session: -autostart on enables it, off disables it, and status checks it. It starts at the next user session; it does not enable lingering or root access. It uses the saved password, available with -print-secret; omit -secret and stdin options when configuring startup. Keep the extracted directory in place.

./yourdesk-client -autostart on -signal wss://desktop.mars-cloud.com:8080/ws
./yourdesk-client -autostart off
./yourdesk-client -autostart status

Extract the package for your architecture and run the Host command below as a regular user. Interactive Shell is unavailable for root.
This package accepts terminal connections only. Add its ID in a recent Mac/Windows YourDesk client, select Terminal and enter the password. Linux graphics and REMOTE are not included. Press Ctrl+C to stop the Host.

CLI: -signal selects the pairing server; -print-uid shows the device ID; -print-secret displays or initially creates the saved password and exits.
Set the password for this run with -secret "your password"; no JSON input is needed. Use at least 6 characters and at most 1000 bytes. The saved password is unchanged. If omitted, the saved password is used or generated on first use. -secret-stdin remains available for automation and cannot be combined with -secret.

env -u DISPLAY -u WAYLAND_DISPLAY ./yourdesk-client -signal wss://desktop.mars-cloud.com:8080/ws -secret "ReplaceWithYourPassword"

./yourdesk-client -print-uid
./yourdesk-client -print-secret

日本語
--------------------
ログイン時の起動には一般ユーザーの systemd セッションが必要です。-autostart on で有効、off で無効、status で確認します。次のユーザーセッションから起動し、linger や root 権限は有効にしません。保存済みパスワードを使います（-print-secret で確認）。設定時は -secret や stdin 引数を外し、展開先を移動しないでください。

./yourdesk-client -autostart on -signal wss://desktop.mars-cloud.com:8080/ws
./yourdesk-client -autostart off
./yourdesk-client -autostart status

対応するアーキテクチャのパッケージを展開し、一般ユーザーで下記 Host コマンドを実行します。root では対話 Shell を利用できません。
本版は命令列接続のみ受け付けます。新版の Mac／Windows YourDesk に ID を登録し、ターミナルを選んでパスワードを入力してください。Linux GUI と REMOTE は含みません。Ctrl+C で停止します。

CLI：-signal は配対サーバー、-print-uid は装置 ID 表示、-print-secret は保存済みパスワードの表示（初回は生成）後に終了します。
今回のパスワードは -secret "パスワード" で指定します。JSON 入力は不要です。6 文字以上、1000 bytes 以下で、保存済みパスワードは変更しません。省略時は保存済みの値を使用し、初回は自動生成します。自動化用の -secret-stdin とは併用できません。

env -u DISPLAY -u WAYLAND_DISPLAY ./yourdesk-client -signal wss://desktop.mars-cloud.com:8080/ws -secret "ReplaceWithYourPassword"

./yourdesk-client -print-uid
./yourdesk-client -print-secret

한국어
--------------------
로그인 시 자동 시작에는 일반 계정의 systemd 사용자 세션이 필요합니다. -autostart on은 켜기, off는 끄기, status는 확인입니다. 다음 사용자 세션부터 시작하며 linger나 root 권한을 켜지 않습니다. 저장된 암호를 사용하며 -print-secret으로 확인합니다. 설정 시 -secret 및 stdin 옵션을 빼고 압축을 푼 폴더를 옮기지 마세요.

./yourdesk-client -autostart on -signal wss://desktop.mars-cloud.com:8080/ws
./yourdesk-client -autostart off
./yourdesk-client -autostart status

컴퓨터 아키텍처에 맞는 패키지를 풀고 일반 사용자로 아래 Host 명령을 실행하세요. root는 대화형 Shell을 사용할 수 없습니다.
이 버전은 터미널 연결만 받습니다. 최신 Mac/Windows YourDesk에 ID를 추가하고 터미널을 선택한 뒤 암호를 입력하세요. Linux GUI 및 REMOTE는 포함되지 않습니다. Ctrl+C로 중지합니다.

CLI: -signal은 연결 서버를 지정하고, -print-uid는 장치 ID를 표시합니다. -print-secret은 저장된 암호를 표시하거나 처음 생성한 뒤 종료합니다.
이번 실행의 암호는 -secret "암호"로 지정합니다. JSON 입력은 필요 없습니다. 최소 6자, 최대 1000 bytes이며 저장된 암호는 변경하지 않습니다. 생략하면 저장된 암호를 사용하고 최초 실행 시 자동 생성합니다. 자동화용 -secret-stdin과 함께 사용할 수 없습니다.

env -u DISPLAY -u WAYLAND_DISPLAY ./yourdesk-client -signal wss://desktop.mars-cloud.com:8080/ws -secret "ReplaceWithYourPassword"

./yourdesk-client -print-uid
./yourdesk-client -print-secret
