## 繁體中文

本版加入 Agent 按需畫面、MCP 畫面契約 v1、AV1 軟體編碼與 macOS 支援，以及 heartbeat-v2 協商。

- MCP 可暫停畫面串流、保留連線與控制，按需擷取 PNG，隨時恢復串流；截圖與串流均支援全螢幕或指定區域。
- 區域滑鼠座標處理 Retina、多螢幕負座標與縮圖比例，使用視野版本拒絕舊影格的座標操作。
- 新增 `get_video_state`、`set_video_mode`、`snapshot`，提供結構化回覆、能力查詢與畫面新鮮度。舊 Host 保留全螢幕串流／截圖，明確回報不支援區域或暫停；初始 paused 連線可降級並提示。
- Windows／macOS 補上 AV1 軟體編碼；macOS 依實測使用 VideoToolbox AV1 硬解或軟解。硬體優先規則不變，同級均衡策略依 AV1 → HEVC → H.264 → JPEG 排序。FFmpeg 維持 LGPL 組態。
- Client 請求 heartbeat-v2，只有 Server 明確接受才啟用；未接受維持舊協議。修正協商、保活及連線失效處理。
- macOS 加入 Siri／捷徑整合（實驗性）：開啟、已儲存站台連線、斷線及狀態查詢；新版 Siri AI 的站台開啟 schema 需相應系統支援。語音與安裝後沙盒端到端驗證尚未完成，不宣稱完整自然語言操作皆可用。
- 編解碼分析隱藏 128×128 與重複尺寸文字，保留內部快速探測並調整欄寬。

包含 macOS ARM64、Windows x64／ARM64 安裝版、Windows x64 免安裝 ZIP、Linux x64／ARM64 命令列 Host ZIP、WinPE x64 實驗版。Android 不包含在本次發布。

驗證：本機 MCP 契約及新舊 P2P Smoke 通過；跨平台建置與 macOS 簽章／公證依發布流程核對。Windows 驅動、Siri 語音及跨機效能仍需實機驗收；Linux／WinPE 不宣稱支援桌面區域串流。

> **防毒偵測說明：部分防毒軟體曾對本程式提出警告。先前僅含 `package main` 與 `func main() {}` 的 Go 空專案，也在使用者提供的 VirusTotal 快照中出現 9/70 偵測；這不是本次套件的掃描結果，也不能證明正式版所有警告都是誤判。作者會持續調查與改善，朝清除告警的方向努力，請勿因此關閉防毒保護。**

[Go 空專案 VirusTotal 報告](https://www.virustotal.com/gui/file/fb906e6611161faafc695ada5042ae82696b88dba3cd1309843d1f42adfe084f) · [Go 官方 FAQ](https://go.dev/doc/faq#virus)

## English

- Adds on-demand Agent screenshots and pausable streaming while retaining the connection and controls. Both support full-screen and region capture, with Retina, display-offset and scaled-image mouse mapping and stale-view protection.
- MCP video contract v1 introduces `get_video_state`, `set_video_mode` and `snapshot`, structured metadata and capability/freshness reporting. Older Hosts retain full-screen operation; unsupported region/pause requests are explicit. An initially paused connection can fall back to streaming with a notice.
- Adds AV1 software encoding on Windows/macOS and measured VideoToolbox decoding on macOS with software fallback. Within the existing hardware priority, Balanced prefers AV1 → HEVC → H.264 → JPEG. FFmpeg remains an LGPL build.
- Negotiates heartbeat-v2 only when explicitly accepted by the Server, retaining legacy fallback. Simplifies codec-analysis rows and column widths.
- Experimental macOS Siri/App Shortcuts integration adds open, saved-site connect, disconnect and status actions. The newer Siri AI site-opening schema requires a supported OS. Voice and installed-extension sandbox end-to-end validation remain incomplete.

Includes macOS ARM64, Windows x64/ARM64 installers, Windows x64 portable ZIP, Linux x64/ARM64 CLI Host ZIPs and experimental WinPE x64. Android is excluded. Local MCP and current/legacy P2P smoke tests passed; Windows driver behavior, Siri voice and cross-device performance still require hardware validation. Desktop region streaming is not claimed for Linux/WinPE.

> **Antivirus notice:** Some scanners have flagged earlier builds. A user-provided VirusTotal snapshot also flagged a minimal empty Go program at 9/70. This is historical evidence, not a scan of these packages or proof that every production alert is a false positive. The author will continue investigating and working to eliminate these alerts. Do not disable antivirus protection.

[Empty Go project report](https://www.virustotal.com/gui/file/fb906e6611161faafc695ada5042ae82696b88dba3cd1309843d1f42adfe084f) · [Official Go FAQ](https://go.dev/doc/faq#virus)

## 日本語

- Agent 向けのストリーム一時停止・再開とオンデマンド PNG を追加しました。接続と操作を維持し、全画面／指定領域、Retina・複数画面・縮小画像の座標変換、古い視野の操作拒否に対応します。
- MCP 画面契約 v1：`get_video_state`、`set_video_mode`、`snapshot` と構造化応答、能力・画像取得方法の確認を追加しました。旧 Host は全画面を維持し、領域／停止の非対応を明示します。接続時の paused は通知付きで全画面ストリームへ戻れます。
- Windows／macOS の AV1 ソフトウェアエンコード、macOS の実測に基づく VideoToolbox デコードを追加しました。既存のハードウェア優先順位を維持し、同条件のバランス設定は AV1 → HEVC → H.264 → JPEG。FFmpeg は LGPL 構成です。
- Server が明示的に受け入れた場合のみ heartbeat-v2 を使用し、それ以外は旧協議を維持します。分析画面の行と列幅も改善しました。
- macOS Siri／ショートカット連携は実験的機能です。起動、保存先への接続、切断、状態確認を提供します。新 Siri AI のサイト表示 schema は対応 OS が必要で、音声とインストール後のサンドボックス動作は未検証です。

macOS ARM64、Windows x64／ARM64 インストーラー、Windows x64 ポータブル ZIP、Linux x64／ARM64 CLI Host、実験版 WinPE x64 を配布します。Android は含みません。ローカル MCP・新旧 P2P Smoke は成功しましたが、Windows ドライバー、Siri 音声、実機間性能は引き続き検証が必要です。Linux／WinPE の領域デスクトップ配信は対象外です。

> **ウイルス対策について：** 過去のビルドには警告があり、利用者提供の空の Go プロジェクトの VirusTotal スナップショットでも 9/70 の検出がありました。今回の配布物の検査結果でも、全警告が誤検知である証明でもありません。作者は警告解消に向けて調査と改善を続けます。ウイルス対策を無効にしないでください。

[空の Go プロジェクトの報告](https://www.virustotal.com/gui/file/fb906e6611161faafc695ada5042ae82696b88dba3cd1309843d1f42adfe084f) · [Go 公式 FAQ](https://go.dev/doc/faq#virus)

## 한국어

- 연결과 제어를 유지하는 Agent 스트림 일시 중지／재개 및 요청 시 PNG 캡처를 추가했습니다. 전체 화면／영역, Retina·다중 화면·축소 이미지 좌표 변환과 오래된 화면의 입력 차단을 지원합니다.
- MCP 화면 계약 v1에 `get_video_state`, `set_video_mode`, `snapshot`, 구조화된 응답과 기능／캡처 방식 조회를 추가했습니다. 이전 Host는 전체 화면을 유지하며 영역／중지 미지원은 명확히 알립니다. paused 연결은 알림과 함께 전체 화면 스트림으로 전환될 수 있습니다.
- Windows／macOS AV1 소프트웨어 인코딩과 macOS의 실측 VideoToolbox 디코딩을 추가했습니다. 기존 하드웨어 우선순위를 유지하며 동급 균형 설정은 AV1 → HEVC → H.264 → JPEG 순서입니다. FFmpeg는 LGPL 구성입니다.
- Server가 명시적으로 수락한 경우에만 heartbeat-v2를 사용하고 이전 프로토콜과 호환됩니다. 분석 화면의 행과 열 너비를 정리했습니다.
- macOS Siri／단축어 통합은 실험 기능입니다. 열기, 저장된 사이트 연결, 연결 해제, 상태 조회를 제공합니다. 새로운 Siri AI 사이트 열기 schema는 지원 OS가 필요하며 음성 및 설치된 확장의 샌드박스 동작은 아직 검증하지 못했습니다.

macOS ARM64, Windows x64／ARM64 설치 프로그램, Windows x64 포터블 ZIP, Linux x64／ARM64 CLI Host와 실험용 WinPE x64를 제공합니다. Android는 제외합니다. 로컬 MCP 및 신구 P2P Smoke는 통과했지만 Windows 드라이버, Siri 음성, 실제 장치 간 성능은 추가 확인이 필요합니다. Linux／WinPE의 데스크톱 영역 스트리밍은 지원 범위에 포함하지 않습니다.

> **백신 안내:** 이전 빌드에서 경고가 있었으며 사용자가 제공한 빈 Go 프로젝트의 VirusTotal 스냅샷도 9/70 탐지를 보였습니다. 이는 이번 패키지의 검사 결과나 모든 경고가 오탐이라는 증거가 아닙니다. 개발자는 경고 해소를 위해 조사와 개선을 계속하겠습니다. 백신 보호를 끄지 마세요.

[빈 Go 프로젝트 보고서](https://www.virustotal.com/gui/file/fb906e6611161faafc695ada5042ae82696b88dba3cd1309843d1f42adfe084f) · [Go 공식 FAQ](https://go.dev/doc/faq#virus)
