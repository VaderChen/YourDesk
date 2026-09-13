## 繁體中文

本版改善傳輸穩定性並加入網路診斷。**目前回報的停滯／斷線問題只發生在 macOS 對 macOS；其他平台尚未觀察到相同問題。**使用者回報更新後單一連線與同時兩個遠端皆維持穩定，較先前版本改善；**尚未完全確認 Mac 對 Mac 停滯／斷線問題已解決，也尚未確認根因**。長時間連線與大檔案傳輸仍需持續驗證。

- Pion WebRTC 4.1.6 → 4.2.20、SCTP 1.8.40 → 1.11.1，並同步更新 DataChannel、DTLS、ICE 與 transport 依賴；不能據此斷言原問題對應特定上游修正。
- 心跳逾時時同時檢查 DataChannel 接收進展；心跳持續失敗且至少 30 秒沒有接收進展，才由此存活檢查關閉連線。仍有其他底層斷線處理，這不是所有情況均等待 30 秒的保證。
- 控制通道壅塞時減少新影像及滑鼠移動流量，保留按鍵與滑鼠按鈕事件；剪貼簿送出改為序列化檢查緩衝上限，避免多個傳檔工作者同時超額送出。可能降低部分環境的檔案吞吐量。
- 「設定 → 關於」加入預設關閉的「網路 Debug」；開啟後才記錄診斷 LOG 並顯示站台雙向封包測試按鈕。由使用者按「開始測試」，自動建立獨立背景連線、暫停影像並校驗雙向資料，顯示實際觀測到的 L1–L6 進展。
- 封包測試需兩端更新並啟用網路 Debug。建議兩端更新，避免舊端沿用較嚴格的存活判斷。
- **本版未加入背景自動重連／斷線續傳**。診斷 LOG、工具鏈核對結果及私人測試資料不隨 Release 上傳。

macOS ARM64 沿用實測的 build 2323 已簽章並通過 Apple 公證 DMG；同版另提供 Windows x64／ARM64 安裝版、Windows x64 免安裝 ZIP、Linux x64／ARM64 CLI Host 及 WinPE x64 實驗版。Android 不包含在本次發行。自動化 Smoke 與跨平台建置不等於所有平台實機驗證。

詳見 [網路 Debug](https://github.com/VaderChen/YourDesk/blob/main/docs/NETWORK-DEBUG.md) 與 [串流停滯及存活判斷](https://github.com/VaderChen/YourDesk/blob/main/docs/STREAM-RECOVERY.md)。

> **防毒偵測說明：部分防毒軟體曾對本程式提出警告。先前僅含 `package main` 與 `func main() {}` 的 Go 空專案，也在使用者提供的 VirusTotal 快照中出現 9/70 偵測；這不是本次套件的掃描結果，也不能證明正式版所有警告都是誤判。作者會持續調查與改善，朝清除告警的方向努力，請勿因此關閉防毒保護。**

[Go 空專案 VirusTotal 報告](https://www.virustotal.com/gui/file/fb906e6611161faafc695ada5042ae82696b88dba3cd1309843d1f42adfe084f) · [Go 官方 FAQ](https://go.dev/doc/faq#virus)

## English

The reported freeze/disconnection issue has so far occurred only between macOS devices; the same issue has not been observed on other platforms. This release improves transport stability and adds optional network diagnostics. The user reports stable operation with one connection and with two simultaneous remote sessions, improving on earlier builds. **The Mac-to-Mac freeze/disconnection issue is not yet confirmed fully resolved, and its root cause remains unconfirmed.** Long-running sessions and large file transfers still need continued validation.

- Upgrades Pion WebRTC to 4.2.20 and SCTP to 1.11.1 with related transport dependencies.
- The liveness monitor considers DataChannel receive progress alongside failed heartbeats. Reduces new screen/mouse-move traffic during control congestion and serializes clipboard send-buffer checks; file throughput may decrease in some environments.
- Network Debug is off by default. Enabling it records diagnostics and reveals the packet-test button. Press Start to open a separate background connection, pause video and verify bidirectional data with observed L1–L6 progress. Both ends must enable Network Debug for this test.
- **Background automatic reconnection and transfer resumption are not included.** Private logs and toolchain audit reports are not published.

Packages: signed/notarized macOS ARM64 DMG, Windows x64/ARM64 installers, Windows x64 portable ZIP, Linux x64/ARM64 CLI Host and experimental WinPE x64. No Android package. Automated smoke checks and builds do not establish complete device-level validation.

Antivirus alerts remain under investigation. An earlier empty Go project was also flagged; this does not prove every alert is a false positive or describe a scan of this release. The author continues working to eliminate alerts. [Empty Go project VirusTotal report](https://www.virustotal.com/gui/file/fb906e6611161faafc695ada5042ae82696b88dba3cd1309843d1f42adfe084f) · [Official Go FAQ](https://go.dev/doc/faq#virus).

## 日本語

これまで報告された停止・切断は macOS 間のみで、他のプラットフォームでは同じ問題は観測されていません。転送の安定性を改善し、任意のネットワーク診断を追加しました。利用者からは単一接続と同時に二つのリモート接続が以前より安定しているとの報告があります。**Mac 間の停止・切断問題が完全に解決したとはまだ確認できておらず、根本原因も未確定です。** 長時間接続と大容量ファイル転送の検証を継続します。

- Pion WebRTC 4.2.20、SCTP 1.11.1 と関連依存関係へ更新しました。
- ハートビート失敗だけでなく DataChannel の受信進捗を確認します。制御チャネル混雑時の新規映像・マウス移動を抑え、クリップボード送信バッファの確認を直列化しました。環境によって転送速度が下がる場合があります。
- Network Debug は初期状態で無効です。有効時のみ診断ログとテストボタンを提供します。「開始」を押すと独立した背景接続で映像を停止し、双方向データを検証して観測した L1–L6 の進捗を表示します。テストには両端での有効化が必要です。
- **背景での自動再接続・中断転送の再開は含みません。** 私的ログやツールチェーン監査結果は公開しません。

署名・Apple 公証済み macOS ARM64、Windows x64／ARM64 インストーラー、Windows x64 ポータブル、Linux x64／ARM64 CLI Host、実験版 WinPE x64 を配布します。Android は対象外です。自動 Smoke とビルド成功は全実機での確認を意味しません。

ウイルス警告は引き続き調査中です。空の Go プロジェクトにも検出がありましたが、全警告が誤検知である証明でも本配布物の検査結果でもありません。作者は警告の解消に向けて改善を続けます。[空の Go プロジェクトの VirusTotal](https://www.virustotal.com/gui/file/fb906e6611161faafc695ada5042ae82696b88dba3cd1309843d1f42adfe084f) · [Go 公式 FAQ](https://go.dev/doc/faq#virus)。

## 한국어

현재 보고된 멈춤／연결 끊김은 macOS 간에서만 발생했으며 다른 플랫폼에서는 같은 문제가 관측되지 않았습니다. 전송 안정성을 개선하고 선택적 네트워크 진단을 추가했습니다. 사용자는 단일 연결과 원격 두 개의 동시 연결이 이전 빌드보다 안정적이라고 보고했습니다. **Mac 간 멈춤／연결 끊김 문제가 완전히 해결되었는지는 아직 확인하지 못했으며 근본 원인도 확정하지 않았습니다.** 장시간 연결과 대용량 파일 전송 검증을 계속합니다.

- Pion WebRTC 4.2.20, SCTP 1.11.1 및 관련 전송 의존성을 업데이트했습니다.
- 하트비트 실패와 함께 DataChannel 수신 진행을 확인합니다. 제어 채널 혼잡 시 새 영상／마우스 이동 전송을 줄이고 클립보드 송신 버퍼 확인을 직렬화했습니다. 일부 환경에서는 파일 전송 속도가 낮아질 수 있습니다.
- Network Debug는 기본적으로 꺼져 있습니다. 켜면 진단 로그와 테스트 버튼을 제공합니다. 시작 버튼을 누르면 별도 백그라운드 연결에서 영상을 일시 중지하고 양방향 데이터를 검증하며 관측된 L1–L6 진행을 표시합니다. 테스트에는 양쪽에서 활성화해야 합니다.
- **백그라운드 자동 재연결과 중단된 파일 전송 재개는 포함하지 않습니다.** 개인 로그와 도구 체인 감사 보고서는 공개하지 않습니다.

서명／Apple 공증된 macOS ARM64, Windows x64／ARM64 설치 프로그램, Windows x64 포터블, Linux x64／ARM64 CLI Host 및 실험용 WinPE x64를 제공합니다. Android는 제외합니다. 자동 Smoke 및 빌드 성공이 모든 실제 장치 검증 완료를 뜻하지는 않습니다.

백신 경고는 계속 조사 중입니다. 빈 Go 프로젝트도 탐지되었지만 모든 경고가 오탐이라는 증거나 이번 배포 파일의 검사 결과는 아닙니다. 개발자는 경고 해소를 위해 계속 개선하겠습니다. [빈 Go 프로젝트 VirusTotal](https://www.virustotal.com/gui/file/fb906e6611161faafc695ada5042ae82696b88dba3cd1309843d1f42adfe084f) · [Go 공식 FAQ](https://go.dev/doc/faq#virus).
