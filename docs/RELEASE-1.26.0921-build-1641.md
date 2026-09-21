# 1.26.0921 build 1641

## 繁體中文

- 修正遠端 Windows Ctrl+Alt+Del：由登入前服務直接提交 SAS，切換安全桌面時保留連線；啟動 App 可確認並授權所需的服務與系統權限。
- 新增可拖曳的全尺寸虛擬鍵盤，提供 Mac、Windows、注音及數字鍵盤；修正鍵盤開啟後的滑鼠操作、標題列按鈕與快捷鍵視窗高度。
- 強制更新開啟後可選擇安裝 GitHub 最新測試版；一般背景更新維持正式版。Apple 低延遲補幀限定 Apple Silicon、macOS 27 以上，改善能力檢查及停用／切換工作階段時的資源回收。
- 修正 Windows 建置中 NASM 靜態函式庫殘留本機路徑造成的驗證失敗。

Windows 10 19045 實測：從 Mac 顯示區域選單選擇 Ctrl+Alt+Del → 遠端，安全選單正常出現，取消後回到桌面，連線保持。遠端測試服務為 build 1616，與本版使用相同修正。既有 Windows 登入前服務需停用再啟用以更新受保護副本，操作期間會中斷連線；只更新 App 不會自動更新該副本。Windows ARM64 尚未實機測試。Apple 補幀通過 macOS 27 合成影格硬體 Smoke，真實遠端場景仍待驗證。

## English

- Fix remote Windows Ctrl+Alt+Del by submitting SAS directly from the pre-login service. Preserve connections across secure-desktop changes and offer a startup permission prompt.
- Add a draggable full-size virtual keyboard with Mac, Windows, Zhuyin and numeric layouts. Fix mouse control while it is open, title-bar buttons and shortcut-dialog sizing.
- Allow installing the latest GitHub prerelease when forced updates are enabled; automatic checks remain on stable releases. Apple low-latency frame interpolation requires Apple Silicon and macOS 27 or later, with improved capability checks and resource cleanup.
- Fix Windows build validation failures caused by local paths embedded in NASM static libraries.

Tested on Windows 10 19045 from a Mac: the remote Ctrl+Alt+Del menu opens the security screen, Cancel returns to the desktop, and the connection stays active. The tested remote service was build 1616 with the same fix. Disable and re-enable an existing Windows pre-login service to refresh its protected copy; this interrupts connections. Updating the app alone does not refresh it. Windows ARM64 has not been tested on hardware. Apple interpolation passed synthetic-frame hardware smoke checks on macOS 27; real remote workloads remain unverified.

## 日本語

- Windows のリモート Ctrl+Alt+Del を修正しました。ログイン前サービスから SAS を直接送信し、セキュアデスクトップへの切り替え中も接続を維持します。起動時に必要な権限を確認できます。
- ドラッグできるフルサイズの仮想キーボードを追加しました。Mac・Windows・注音・数字配列に対応し、表示中のマウス操作、タイトルバーのボタン、ショートカット確認画面の高さを修正しました。
- 強制更新を有効にすると GitHub の最新プレリリースを選択できます。通常の自動更新は正式版を使用します。Apple 低遅延フレーム補間は Apple Silicon・macOS 27 以降に限定し、機能確認とリソース解放を改善しました。
- NASM 静的ライブラリにローカルパスが残り、Windows のビルド検証が失敗する問題を修正しました。

Windows 10 19045 と Mac で実機確認済みです。リモート Ctrl+Alt+Del で安全画面が表示され、キャンセルでデスクトップに戻り、接続も維持されました。検証したサービスは本版と同じ修正を含む build 1616 です。既存の Windows ログイン前サービスは無効化後に再度有効化して更新してください。この操作中は接続が切れます。App の更新だけではサービスのコピーは更新されません。Windows ARM64 は実機未検証です。Apple 補間は macOS 27 の合成フレームによるハードウェア Smoke を通過しましたが、実際のリモート環境は未検証です。

## 한국어

- Windows 원격 Ctrl+Alt+Del을 수정했습니다. 로그인 전 서비스에서 SAS를 직접 보내며 보안 데스크톱 전환 중에도 연결을 유지합니다. 앱 시작 시 필요한 권한을 확인할 수 있습니다.
- 드래그 가능한 전체 크기 가상 키보드를 추가했습니다. Mac, Windows, 주음 및 숫자 배열을 제공하며 키보드 표시 중 마우스 조작, 제목 표시줄 버튼, 단축키 대화상자 높이를 수정했습니다.
- 강제 업데이트를 켜면 GitHub 최신 사전 릴리스를 선택할 수 있습니다. 일반 자동 업데이트는 정식 버전을 사용합니다. Apple 저지연 프레임 보간은 Apple Silicon과 macOS 27 이상으로 제한하고 기능 확인 및 자원 정리를 개선했습니다.
- NASM 정적 라이브러리에 로컬 경로가 남아 Windows 빌드 검증이 실패하던 문제를 수정했습니다.

Windows 10 19045와 Mac에서 실기 검증했습니다. 원격 Ctrl+Alt+Del로 보안 화면이 나타나고 취소하면 바탕 화면으로 돌아오며 연결이 유지됩니다. 테스트한 서비스는 같은 수정 사항을 포함한 build 1616입니다. 기존 Windows 로그인 전 서비스는 껐다가 다시 켜서 보호된 사본을 갱신해야 하며 이 과정에서 연결이 끊깁니다. 앱 업데이트만으로는 서비스 사본이 갱신되지 않습니다. Windows ARM64는 실기 미검증입니다. Apple 보간은 macOS 27 합성 프레임 하드웨어 Smoke를 통과했지만 실제 원격 환경은 추가 검증이 필요합니다.
