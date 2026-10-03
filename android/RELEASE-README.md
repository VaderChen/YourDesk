## 繁體中文

既有版本首次須手動安裝本版；相同正式簽章及 applicationId 可直接覆蓋更新，不需卸載。之後在首頁前景閒置時自動偵測與下載，連線或進入背景時停止；下載完成後由使用者選擇安裝，必要時允許 YourDesk 安裝來源，最後仍由 Android 系統確認。

適用 Android 8.0 以上的 arm64 手機。先解壓縮 ZIP，再開啟其中的 `.apk` 安裝；若系統詢問，允許此次使用的檔案管理器安裝 App。在遠端電腦執行 YourDesk Host，於手機輸入遠端 ID／IP 或選擇站台，再輸入連線密碼。

這是操作遠端電腦的 Viewer，沒有手機被控模式。舊測試版若簽章不同，不能直接覆蓋；卸載會清除站台與記憶密碼，請先保留必要資料並查閱對應版本的[發行說明](https://github.com/VaderChen/YourDesk/releases)。

`SHA256SUMS` 可校驗解壓縮後的檔案；`BUILD.json` 保留 APK 建置紀錄。使用條件見 `LICENSE.md`。

## English

Existing users must install this version manually once. The same official signing certificate and applicationId allow updating without uninstalling. Later updates are detected and downloaded while the home screen is idle in the foreground; connecting or moving to the background stops this work. Choose to install after download, allow YourDesk as an installation source if needed, then confirm in Android.

For arm64 phones running Android 8.0 or later. Extract the ZIP, then open the included `.apk` to install it. If prompted, allow the file manager you are using to install apps. Run YourDesk Host on the remote computer, enter its ID/IP or select a saved site on your phone, then enter the connection password.

This is a viewer for controlling a remote computer; it does not let another device control the phone. Older test builds signed with a different key cannot be updated directly. Uninstalling deletes saved sites and passwords; preserve the details you need and read the corresponding [release notes](https://github.com/VaderChen/YourDesk/releases) first.

Use `SHA256SUMS` to verify the extracted files. `BUILD.json` contains the APK build record. See `LICENSE.en.md` for the terms of use.

## 日本語

既存の利用者は最初に本版を一度手動インストールしてください。同じ正式署名と applicationId を使用し、アンインストールせずに更新できます。以後はホーム画面が前面で待機中に検出・ダウンロードし、接続時やバックグラウンド移行時には停止します。完了後にインストールを選び、必要に応じて YourDesk からのインストールを許可し、Android の確認画面で承認してください。

Android 8.0 以降の arm64 スマートフォン向けです。ZIP を展開し、中の `.apk` を開いてインストールしてください。確認が表示された場合は、使用中のファイルマネージャーからのインストールを許可してください。リモートコンピューターで YourDesk Host を起動し、スマートフォンで ID／IP を入力するか接続先を選び、接続パスワードを入力します。

リモートコンピューターを操作する Viewer であり、スマートフォン自体を遠隔操作する機能はありません。署名鍵が異なる旧テスト版は直接更新できません。アンインストールすると保存済みの接続先とパスワードが消去されます。必要な情報を保存し、該当バージョンの[リリースノート](https://github.com/VaderChen/YourDesk/releases)を先に確認してください。

`SHA256SUMS` で展開後のファイルを検証できます。`BUILD.json` は APK のビルド記録です。利用条件は `LICENSE.ja.md` を参照してください。

## 한국어

기존 사용자는 이번 버전을 한 번 수동 설치해야 합니다. 같은 정식 서명과 applicationId를 사용하므로 삭제하지 않고 업데이트할 수 있습니다. 이후에는 홈 화면이 전경에서 대기 중일 때 감지·다운로드하며 연결하거나 백그라운드로 이동하면 중단합니다. 다운로드 후 설치를 선택하고 필요하면 YourDesk를 설치 출처로 허용한 뒤 Android에서 확인하세요.

Android 8.0 이상을 사용하는 arm64 휴대폰용입니다. ZIP 압축을 푼 뒤 포함된 `.apk`를 열어 설치하세요. 시스템에서 요청하면 사용 중인 파일 관리자의 앱 설치를 허용하세요. 원격 컴퓨터에서 YourDesk Host를 실행하고, 휴대폰에서 ID/IP를 입력하거나 저장된 접속 대상을 선택한 뒤 연결 비밀번호를 입력하세요.

원격 컴퓨터를 제어하는 Viewer이며 휴대폰 자체를 원격 제어하는 기능은 없습니다. 서명 키가 다른 이전 테스트 버전은 바로 업데이트할 수 없습니다. 앱을 삭제하면 저장된 접속 대상과 비밀번호도 지워집니다. 필요한 정보를 보관하고 해당 버전의 [릴리스 노트](https://github.com/VaderChen/YourDesk/releases)를 먼저 확인하세요.

`SHA256SUMS`로 압축을 푼 파일을 검증할 수 있습니다. `BUILD.json`에는 APK 빌드 기록이 들어 있습니다. 이용 조건은 `LICENSE.ko.md`를 참고하세요.
