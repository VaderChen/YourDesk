# 1.26.0919 build 0519

## 繁體中文

- 調整 Mac 大檔案貼上的資料交付頻率，減少累積等待；Finder 進度改善程度仍待實測。
- 系統快捷鍵可選本機、遠端或取消；一般複製貼上維持原有操作。
- 新增四語更新重點、修正更新重點保存與 Mac 簽章處理，Windows 免安裝版優先使用 ZIP 更新。

只需更新貼上端 Mac 即可套用資料交付調整。每次最多讀取 32 KiB，有資料交付時每 100 ms 刷新 HTTP 緩衝，保留既有背壓；高延遲網路的峰值速度可能降低。Finder 自身仍控制進度刷新。

驗證：Go 與 WebView smoke 通過，包含首段資料交付、Range／HEAD。尚未實測 Finder 進度改善、Windows 快捷鍵及安裝後更新流程。Ctrl+Alt+Del 不支援遠端合成。七個桌面／CLI／WinPE 套件，不含 Android。

## English

- Deliver pasted file data more frequently on Mac to reduce buffering delays; Finder progress improvements still need real-world verification.
- Choose local, remote or cancel for system shortcuts; normal copy and paste are unchanged.
- Add localized update highlights, fix persistence and Mac signing, and prefer ZIP updates for Windows portable editions.

Update the receiving Mac to apply the delivery change. Reads are capped at 32 KiB, with HTTP buffers flushed every 100 ms when data is delivered. Existing backpressure remains; peak throughput may decrease on high-latency networks. Finder controls its own progress refresh.

Validation: Go and WebView smoke checks passed, including early data delivery and Range/HEAD handling. Finder progress, Windows shortcuts and the post-install update flow still require real-device testing. Remote Ctrl+Alt+Del synthesis is unsupported. Seven desktop/CLI/WinPE packages; no Android release.

## 日本語

- Mac へのファイル貼り付け時のデータ受け渡し間隔を短縮しました。Finder の進捗改善は実機での確認が必要です。
- システムショートカットの対象を本機・リモート・キャンセルから選べます。通常のコピーとペーストは従来どおりです。
- 多言語の更新内容表示と保存処理、Mac の署名処理を改善し、Windows ポータブル版は ZIP 更新を優先します。

受信側の Mac を更新すると適用されます。読み取りは最大 32 KiB とし、データ受け渡し時に 100 ms 間隔で HTTP バッファをフラッシュします。既存のバックプレッシャーを維持するため、高遅延回線では最大速度が低下する場合があります。進捗表示の更新は Finder 自身が制御します。

検証：先頭データの受け渡しと Range／HEAD を含む Go・WebView smoke が通過しました。Finder の進捗改善、Windows ショートカット、インストール後の更新処理は実機検証が必要です。リモート Ctrl+Alt+Del の合成は非対応です。デスクトップ／CLI／WinPE の7パッケージを配布し、Android は含みません。

## 한국어

- Mac에 파일을 붙여넣을 때 데이터를 더 자주 전달하도록 조정했습니다. Finder 진행 표시 개선은 실제 환경에서 확인이 필요합니다.
- 시스템 단축키의 대상을 로컬, 원격 또는 취소로 선택합니다. 일반 복사와 붙여넣기는 유지합니다.
- 다국어 업데이트 내용과 저장 및 Mac 서명 처리를 개선하고 Windows 포터블 버전은 ZIP 업데이트를 우선합니다.

수신 측 Mac을 업데이트하면 적용됩니다. 읽기는 최대 32 KiB이며 데이터 전달 시 100 ms 간격으로 HTTP 버퍼를 비웁니다. 기존 역압을 유지하므로 지연이 큰 네트워크에서는 최대 속도가 낮아질 수 있습니다. 진행 표시 갱신은 Finder가 제어합니다.

검증: 첫 데이터 전달과 Range／HEAD 처리를 포함한 Go 및 WebView smoke 검사를 통과했습니다. Finder 진행 표시, Windows 단축키와 설치 후 업데이트 흐름은 실제 기기 테스트가 필요합니다. 원격 Ctrl+Alt+Del 합성은 지원하지 않습니다. 데스크톱／CLI／WinPE 패키지 7개이며 Android는 포함하지 않습니다.

