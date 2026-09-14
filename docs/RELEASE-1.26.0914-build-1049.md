## 繁體中文

本版包含 build 2323 之後的串流穩定修正、剪貼簿流控、封包分析改善與安裝包瘦身。

- 已處理 Viewer 解碼佇列滿載時阻塞 WebRTC DataChannel 接收回呼的問題；一般與背景 Viewer 均採非阻塞影格交接，保留既有影格恢復流程。
- 剪貼簿與檔案使用接收額度流控，接收較慢時讓傳送端等待；讀取回覆與完成確認使用獨立額度，保留內容與順序。舊文字剪貼簿改由有序工作者處理，避免系統剪貼簿寫入拖住心跳。完整流控需兩端更新；部分情境傳輸可能較慢。
- 「網路 Debug」更名為「封包分析」。日誌採有界背景佇列與批次磁碟同步，滿載只略過診斷紀錄；統計取樣最多等 100 毫秒，避免測試結束無限等待。由使用者按「開始測試」才建立背景連線。
- macOS DMG 改用 LZMA；相同 build 1007 內容比較由 62.5 MiB 降至 50.7 MiB（約 19%），此為壓縮格式比較，非本版套件最終大小。Viewer 拆除不必要的管理介面依賴，另減少約 1.1 MiB；ZIP 提高壓縮等級。編解碼、AI 模型、第三方來源與授權均保留。
- 修正 macOS 建置大量重複 rpath 警告。

先前回報的停滯／斷線目前只見於 macOS 對 macOS；其他平台尚未觀察到相同問題。已修正可重現的解碼佇列阻塞，但不據此認定所有斷線均已解決。長時間連線與跨機大檔案貼上仍需持續驗證。**本版沒有背景自動重連／斷線續傳。**

驗證包含本機 WebRTC 雙向資料與順序比對、慢速接收、慢速日誌、統計逾時、競態檢查及 macOS Viewer Smoke；跨平台建置不等於所有平台的實機功能驗證。診斷 LOG、工具鏈核對結果與私人測試資料不隨 Release 上傳。

本版 macOS DMG 實際大小為 **52.1 MiB**。

套件：macOS arm64 簽章／Apple 公證 DMG、Windows x64／arm64 安裝版、Windows x64 免安裝 ZIP、Linux x64／arm64 CLI Host、WinPE x64 實驗版。Android 不包含於本次發行。

部分防毒軟體曾對本程式提出警告；先前 Go 空專案也在使用者提供的 VirusTotal 快照中出現 9/70 偵測。這不是本版套件掃描結果，也不能證明所有警告都是誤判。作者會持續調查改善，朝清除告警的方向努力。[空專案 VirusTotal 報告](https://www.virustotal.com/gui/file/fb906e6611161faafc695ada5042ae82696b88dba3cd1309843d1f42adfe084f) · [Go 官方 FAQ](https://go.dev/doc/faq#virus)。

## English

- Fixes the reproducible Viewer decode-queue blockage in WebRTC receive callbacks using nonblocking frame handoff.
- Adds receiver credits for clipboard/file transfers. Slow receivers make senders wait while preserving data and order; read replies and completion acknowledgements have separate credits. Legacy clipboard writes run on an ordered worker. Update both ends for full flow control; throughput may decrease.
- Renames Network Debug to Packet analysis. Bounded background logging and 100 ms statistics waits reduce diagnostic blocking risks. Tests start only after pressing Start.
- Uses LZMA for macOS DMGs and stronger ZIP compression. Identical build 1007 contents measured 62.5 → 50.7 MiB (about 19%); this is a format comparison, not this release's final size. Removing the Viewer's unnecessary UI dependency separately saves about 1.1 MiB. Codecs, AI models, source archives and licenses remain included.
- Fixes duplicate macOS rpath warnings.

Previously reported freezes occurred only on Mac-to-Mac connections. The reproducible queue defect is fixed, but long sessions and cross-device large-file pastes still need validation. **No automatic background reconnect or transfer resumption.** Local WebRTC, slow-receiver/logging, timeout, race and Viewer smoke checks do not establish real-device coverage on every platform. Private logs and toolchain audit reports are excluded.

This release’s macOS DMG is **52.1 MiB**.

Packages: signed/notarized macOS arm64 DMG; Windows x64/arm64 installers; Windows x64 portable; Linux x64/arm64 CLI Host; experimental WinPE x64. No Android package.

An empty Go project also triggered detections in an earlier report. This neither proves all warnings are false positives nor represents a scan of these assets. The author continues working to clear alerts. [Empty-project VirusTotal](https://www.virustotal.com/gui/file/fb906e6611161faafc695ada5042ae82696b88dba3cd1309843d1f42adfe084f) · [Official Go FAQ](https://go.dev/doc/faq#virus).

## 日本語

- Viewer のデコードキュー満杯時に WebRTC 受信処理を止める問題を、非ブロッキングのフレーム受け渡しで修正しました。
- クリップボード／ファイル転送に受信側クレジットを導入しました。低速時は内容と順序を保持して送信を待機し、読み取り応答と完了確認は別枠で扱います。旧テキスト貼り付けも順序付きワーカーで処理します。完全なフロー制御には両端の更新が必要で、転送速度が下がる場合があります。
- 「Network Debug」を「パケット分析」に変更しました。上限付きの背景ログと 100 ms の統計待機上限で診断のブロックを軽減します。テストは「開始」を押してから実行します。
- macOS DMG を LZMA に変更し、ZIP 圧縮率も向上しました。同一 build 1007 内容の比較は 62.5 → 50.7 MiB（約 19% 減）で、本リリースの最終サイズではありません。Viewer の不要な UI 依存除去で別途約 1.1 MiB 削減しました。コーデック、AI モデル、ソース、ライセンスは維持します。
- macOS の rpath 重複警告を修正しました。

これまでの停止報告は Mac 間のみです。再現可能なキュー問題は修正しましたが、長時間接続と端末間の大容量貼り付けは継続検証が必要です。**背景自動再接続・転送再開は含みません。** ローカル WebRTC、低速受信／ログ、タイムアウト、競合検査、Viewer Smoke は全プラットフォームの実機確認を意味しません。私的ログやツールチェーン監査は公開しません。

今回の macOS DMG は **52.1 MiB** です。

macOS arm64 署名／公証済み DMG、Windows x64／arm64 インストーラー、Windows x64 ポータブル、Linux x64／arm64 CLI Host、実験版 WinPE x64 を配布します。Android は対象外です。

空の Go プロジェクトにも検出例がありますが、全警告の誤検知を証明するものでも今回の配布物の検査結果でもありません。作者は警告解消に向け改善を続けます。[空プロジェクト VirusTotal](https://www.virustotal.com/gui/file/fb906e6611161faafc695ada5042ae82696b88dba3cd1309843d1f42adfe084f) · [Go 公式 FAQ](https://go.dev/doc/faq#virus)。

## 한국어

- Viewer 디코딩 큐가 가득 찼을 때 WebRTC 수신 처리를 막던 문제를 비차단 프레임 전달로 수정했습니다.
- 클립보드와 파일 전송에 수신 크레딧을 도입했습니다. 느린 수신 시 내용과 순서를 유지하며 송신이 기다리고, 읽기 응답과 완료 확인은 별도 크레딧을 사용합니다. 기존 텍스트 붙여넣기도 순서가 있는 작업자로 처리합니다. 완전한 흐름 제어에는 양쪽 업데이트가 필요하며 전송이 느려질 수 있습니다.
- Network Debug를 패킷 분석으로 변경했습니다. 제한된 백그라운드 로그와 100 ms 통계 대기로 진단 자체의 차단 위험을 줄입니다. 시작 버튼을 눌러야 테스트합니다.
- macOS DMG에 LZMA, ZIP에 높은 압축 수준을 적용했습니다. 동일한 build 1007 내용 비교는 62.5 → 50.7 MiB(약 19% 감소)이며 이번 배포 파일의 최종 크기는 아닙니다. Viewer의 불필요한 UI 의존성 제거로 별도 약 1.1 MiB를 줄였습니다. 코덱, AI 모델, 소스와 라이선스는 유지합니다.
- macOS rpath 중복 경고를 수정했습니다.

기존 멈춤 보고는 Mac 간에서만 관측되었습니다. 재현 가능한 큐 문제는 수정했지만 장시간 연결과 기기 간 대용량 붙여넣기는 계속 검증해야 합니다. **자동 백그라운드 재연결이나 전송 재개는 없습니다.** 로컬 WebRTC, 느린 수신／로그, 시간 초과, 경쟁 검사 및 Viewer Smoke가 모든 플랫폼의 실제 기기 검증을 의미하지는 않습니다. 개인 로그와 도구 체인 감사 보고서는 공개하지 않습니다.

이번 macOS DMG는 **52.1 MiB**입니다.

서명／공증된 macOS arm64 DMG, Windows x64／arm64 설치 프로그램, Windows x64 포터블, Linux x64／arm64 CLI Host, 실험용 WinPE x64를 제공합니다. Android는 제외합니다.

빈 Go 프로젝트도 탐지된 사례가 있지만 모든 경고가 오탐이라는 증거나 이번 배포물의 검사 결과는 아닙니다. 개발자는 경고 해소를 위해 계속 개선합니다. [빈 프로젝트 VirusTotal](https://www.virustotal.com/gui/file/fb906e6611161faafc695ada5042ae82696b88dba3cd1309843d1f42adfe084f) · [Go 공식 FAQ](https://go.dev/doc/faq#virus).
