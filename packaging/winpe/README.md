# YourDesk WinPE

## 繁體中文

使用具備網卡驅動的 Windows 10／11 WinPE x64，解壓後執行 start-yourdesk.cmd。視窗顯示本次臨時 ID，按 Show 查看臨時密碼。
由一般 YourDesk Client 選擇桌面連線，Tailcat 必須關閉。Stop 中斷連線，Start 重新接受連線，關閉視窗即結束。
ID 與密碼每次啟動都會更換，沒有自訂密碼參數。不支援命令列、剪貼簿、檔案傳送或自動更新。此為實驗性功能，尚待 WinPE 實機驗證。
可用參數：-signal 配對伺服器；-fps 1～20；-quality 20～90。

```bat
start-yourdesk.cmd -signal wss://desktop.mars-cloud.com:8080/ws -fps 8 -quality 60
```

## English

Use Windows 10/11 based WinPE x64 with network drivers. Extract and run start-yourdesk.cmd. The window shows a temporary ID; click Show for the temporary password.
Connect in Desktop mode from YourDesk with Tailcat disabled. Stop disconnects, Start accepts connections again, and closing the window exits.
The ID and password change on each launch; no custom password flag is available. Terminal, clipboard, file transfer and automatic updates are unsupported. Experimental; real WinPE validation is pending.
Options: -signal pairing server; -fps 1–20; -quality 20–90.

```bat
start-yourdesk.cmd -signal wss://desktop.mars-cloud.com:8080/ws -fps 8 -quality 60
```

## 日本語

ネットワークドライバーを備えた Windows 10／11 ベースの WinPE x64 で展開し、start-yourdesk.cmd を実行します。一時 ID が表示され、Show でパスワードを確認できます。
通常の YourDesk からデスクトップ接続し、Tailcat は無効にします。Stop で切断、Start で再開、ウィンドウを閉じると終了します。
ID とパスワードは起動ごとに変わり、パスワード指定オプションはありません。ターミナル、クリップボード、ファイル転送、自動更新は非対応です。実験機能で、WinPE 実機検証は未完了です。
オプション：-signal 配対サーバー、-fps 1～20、-quality 20～90。

```bat
start-yourdesk.cmd -signal wss://desktop.mars-cloud.com:8080/ws -fps 8 -quality 60
```

## 한국어

네트워크 드라이버가 있는 Windows 10/11 기반 WinPE x64에서 압축을 풀고 start-yourdesk.cmd를 실행하세요. 임시 ID가 표시되며 Show로 암호를 확인합니다.
일반 YourDesk에서 데스크톱으로 연결하고 Tailcat을 끄세요. Stop은 연결 중지, Start는 다시 연결 허용이며 창을 닫으면 종료합니다.
ID와 암호는 실행할 때마다 변경되며 사용자 지정 암호 옵션은 없습니다. 터미널, 클립보드, 파일 전송 및 자동 업데이트는 지원하지 않습니다. 실험 기능이며 실제 WinPE 검증은 아직 완료되지 않았습니다.
옵션: -signal 연결 서버, -fps 1～20, -quality 20～90.

```bat
start-yourdesk.cmd -signal wss://desktop.mars-cloud.com:8080/ws -fps 8 -quality 60
```
