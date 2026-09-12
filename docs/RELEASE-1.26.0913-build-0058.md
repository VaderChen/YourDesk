## 繁體中文

> **防毒偵測說明：目前部分防毒軟體會對 YourDesk 發出告警。我們測試的空 Go 專案也出現告警，Go 官方 FAQ 亦說明 Go 程式可能遭誤判；但目前尚未確認正式版的所有告警都是誤報。作者會持續調查與改善，朝消除這些告警的方向努力。請自行評估後下載使用，勿為此關閉防毒保護。**

防毒告警調查更新：我們建立了一個僅有 `package main` 與 `func main() {}`、沒有引用 YourDesk 或第三方套件的 Go 空專案。其 Windows x64 執行檔仍出現 **9／70 家引擎告警**（使用者提供的 VirusTotal 掃描快照，數字可能隨後續分析改變）。詳見 [空專案檢測報告](https://www.virustotal.com/gui/file/fb906e6611161faafc695ada5042ae82696b88dba3cd1309843d1f42adfe084f)。這表示告警不需要遠端控制或防火牆穿透功能才會發生。

[Go 官方 FAQ：為什麼防毒軟體認為我的 Go 程式受到感染？](https://go.dev/doc/faq#virus) 也說明 Go 程式可能遭防毒誤判。不過，這不代表正式版的全部告警已確認為誤報。**作者依舊會持續調查與改善，朝消除這些病毒告警的方向努力。**

新增 Windows x64 免安裝 ZIP：完整解壓縮後執行 `YourDesk.exe`，仍需 WebView2 Runtime。與本版安裝程式使用相同執行檔，包含必要 DLL 與授權資料。

目前已確認本版 Windows x64 `YourDesk.exe` 有 7／70 家引擎告警；Linux x64／ARM64 `yourdesk-client` 分別有 1／64、1／62 家告警，Microsoft 判定為 `Trojan:Script/Wacatac.C!ml`。檢測數字為 2026-09-13 查核時的快照，後續可能改變。上述三個執行檔均可由目前原始碼重新建置成相同雜湊，但此結果不等同安全驗證或誤報確認。macOS 本次未回報告警，亦不代表所有功能已經過安全驗收。

- 新增「編解碼分析」：macOS、Windows 的編碼與解碼分開執行；解碼使用固定樣本，不依賴本機編碼成功。深度測試長按一秒開始，完成後收起進度。
- Windows 加入 FFmpeg LGPL 的 H.264／HEVC／AV1 軟解、原生格式矩陣、CPU／GPU 與 D3D11／D3D12 分析；正式 AV1 硬編依硬體與驅動實測能力啟用。原生 HEVC 編碼測試成功不代表正式 Windows RGBA 串流已有 HEVC 編碼後端。
- 「串流方式」新增「傳輸策略」：均衡（預設）、低延遲、省頻寬。編碼非 AUTO 時停用，保留所選策略；移除原本兩個自動配置按鈕。
- 候選依雙端硬體、編碼端硬體、實測尺寸及策略排序；單一編碼失敗後繼續嘗試其他可用候選。FPS、碼率與 GOP 保留使用者設定。
- 修正 Windows 缺少 DLL 相依檔造成的無畫面啟動失敗，服務安裝一併複製所需動態庫；啟動器補充失敗訊息。
- 改善差分與像素轉換、實際傳輸位元組統計及影像速率限制，整理分析表格欄寬、字體、標題與對齊。
- 納入服務帳號的 Shell／檔案入口限制；Windows SYSTEM 情境仍待實機驗收。

提供 macOS ARM64、Windows x64／ARM64 安裝版、Linux x64／ARM64 CLI，以及 WinPE x64 實驗版。Android 不包含於本次原始碼提交或 Release。

驗證：產品 Go 套件測試與相關 Smoke 通過；Windows 跨架構編譯與 DLL 相依檢查通過；macOS App／DMG 已簽章、公證。Windows 驅動、完整連線效能與服務權限的實機驗收仍有未完成項目。傳輸策略為能力排序，不宣稱已量測出所有裝置的最佳效能。

## English

> **Antivirus notice: some antivirus products currently flag YourDesk. Our empty Go test project also triggered detections, and the official Go FAQ describes possible false positives for Go programs. However, not all detections in the release have been confirmed as false positives. The author will continue investigating and improving the project to address these alerts. Assess the risk before downloading; do not disable antivirus protection.**

Antivirus investigation update: a minimal Go project containing only `package main` and `func main() {}`, with no YourDesk or third-party imports, still produced a Windows x64 executable flagged by **9/70 engines** in a user-provided VirusTotal snapshot. Counts may change. See the [empty-project scan report](https://www.virustotal.com/gui/file/fb906e6611161faafc695ada5042ae82696b88dba3cd1309843d1f42adfe084f). Remote-control or firewall-traversal functionality is therefore not required for a detection to occur.

The [official Go FAQ on antivirus detections](https://go.dev/doc/faq#virus) also explains that Go programs can be falsely flagged. This does not establish that all release detections are false positives. **The author will continue investigating and improving the project, working toward eliminating these antivirus alerts.**

Added a Windows x64 portable ZIP: extract all files and run `YourDesk.exe`. WebView2 Runtime is required. It contains the same binaries as this release’s installer, with required DLLs and licenses.

As checked on September 13, 2026, the Windows x64 launcher has 7/70 detections. Linux x64 and ARM64 clients have 1/64 and 1/62, respectively, with Microsoft reporting `Trojan:Script/Wacatac.C!ml`. All three binaries reproduce byte-for-byte from the current source. This establishes reproducibility, not safety or a confirmed false positive. No macOS alert was reported in this round; that is not a full security assessment.

This release separates encoding and decoding tests, adds a one-second hold to start deep testing, and brings FFmpeg LGPL H.264/HEVC/AV1 software decoding to Windows. Native Windows format and D3D11/D3D12 analysis is expanded. Transmission strategies offer Balanced (default), Low latency and Save bandwidth, enabled only with AUTO encoding. Hardware and measured-size evidence take priority; a failed encoder no longer disables every hardware candidate. Existing FPS, bitrate and GOP settings are preserved. Windows DLL packaging and startup error reporting are fixed, with analysis layout and streaming pipeline improvements.

The release includes macOS ARM64, Windows x64/ARM64 installers, Linux x64/ARM64 CLI ZIPs and an experimental WinPE x64 ZIP. Android is excluded. Product tests and smoke checks passed, Windows builds and DLL dependencies were checked, and macOS App/DMG signing and notarization passed. Windows driver behavior, end-to-end performance and SYSTEM service restrictions still require hardware validation. Successful native HEVC encoding tests do not imply a production Windows RGBA HEVC encoder.

## 日本語

> **ウイルス対策ソフトに関する説明：現在、一部のウイルス対策ソフトが YourDesk に警告を出しています。テストした空の Go プロジェクトでも検出があり、Go 公式 FAQ にも誤検知の可能性が説明されています。ただし、正式版のすべての警告が誤検知と確認されたわけではありません。作者は警告の解消に向けて調査と改善を続けます。リスクをご判断のうえダウンロードし、ウイルス対策を無効にしないでください。**

ウイルス対策の調査更新：`package main` と `func main() {}` のみで、YourDesk やサードパーティのパッケージをインポートしない Go の空プロジェクトでも、Windows x64 実行ファイルが **9/70 エンジンで検出**されました。利用者提供の VirusTotal スナップショットであり、数値は変わる可能性があります。[空プロジェクトの検出レポート](https://www.virustotal.com/gui/file/fb906e6611161faafc695ada5042ae82696b88dba3cd1309843d1f42adfe084f)をご覧ください。リモート操作やファイアウォール越えの機能がなくても検出は発生しています。

[Go 公式 FAQ](https://go.dev/doc/faq#virus) にも Go プログラムの誤検知について説明があります。ただし、正式版の全警告が誤検知と確認されたわけではありません。**作者は引き続き調査と改善を行い、これらのウイルス警告の解消に向けて取り組みます。**

Windows x64 ポータブル ZIP を追加しました。全て展開して `YourDesk.exe` を実行してください。WebView2 Runtime が必要です。同版インストーラーと同じ実行ファイル、必要な DLL とライセンスを同梱します。

2026年9月13日の確認では、Windows x64 ランチャーは7/70、Linux x64／ARM64 は1/64、1/62の検出です。Linux の Microsoft 判定は `Trojan:Script/Wacatac.C!ml` です。3ファイルとも現在のソースから同じバイナリを再現できましたが、安全性や誤検知を証明するものではありません。macOS は今回警告の報告がありませんが、完全な安全性検証ではありません。

エンコードとデコードを独立テストに分離し、1秒長押しの詳細テストを追加しました。Windows は FFmpeg LGPL による H.264／HEVC／AV1 ソフトウェアデコード、ネイティブ形式、D3D11／D3D12 分析に対応します。AUTO 時のみ選べる伝送方針はバランス（既定）、低遅延、帯域節約です。ハードウェア能力と実測サイズを優先し、失敗した候補から次の候補へ移行します。FPS、ビットレート、GOP の設定は保持します。Windows の DLL 同梱と起動失敗表示、分析画面と転送処理を改善しました。

macOS ARM64、Windows x64／ARM64、Linux x64／ARM64 CLI、実験版 WinPE x64 を配布し、Android は含みません。製品テスト、Smoke、クロスコンパイル、DLL 検査、macOS 署名・公証を確認しました。Windows 実機のドライバー、接続性能、SYSTEM サービス制限は引き続き検証が必要です。ネイティブ HEVC テスト成功は正式 RGBA ストリームの HEVC エンコード対応を意味しません。

## 한국어

> **백신 안내: 현재 일부 백신이 YourDesk에 경고를 표시합니다. 테스트한 빈 Go 프로젝트에서도 탐지가 발생했으며 Go 공식 FAQ도 Go 프로그램의 오탐 가능성을 설명합니다. 다만 정식 버전의 모든 경고가 오탐으로 확인된 것은 아닙니다. 개발자는 경고 해소를 위해 조사와 개선을 계속하겠습니다. 위험을 판단한 후 다운로드하고 백신 보호를 끄지 마세요.**

백신 조사 업데이트: `package main`과 `func main() {}`만 있고 YourDesk 또는 타사 패키지를 가져오지 않는 빈 Go 프로젝트의 Windows x64 실행 파일도 **9/70 엔진에서 탐지**되었습니다. 사용자가 제공한 VirusTotal 스냅샷이며 수치는 달라질 수 있습니다. [빈 프로젝트 검사 보고서](https://www.virustotal.com/gui/file/fb906e6611161faafc695ada5042ae82696b88dba3cd1309843d1f42adfe084f)를 참고하세요. 원격 제어나 방화벽 통과 기능이 없어도 탐지가 발생한다는 뜻입니다.

[Go 공식 FAQ](https://go.dev/doc/faq#virus)에서도 Go 프로그램의 오탐 가능성을 설명합니다. 그러나 정식 버전의 모든 경고가 오탐으로 확인된 것은 아닙니다. **개발자는 이러한 바이러스 경고를 해소하기 위해 계속 조사하고 개선하겠습니다.**

Windows x64 무설치 ZIP을 추가했습니다. 전체 압축 해제 후 `YourDesk.exe`를 실행하세요. WebView2 Runtime이 필요합니다. 같은 버전 설치 프로그램의 실행 파일과 필수 DLL 및 라이선스를 포함합니다。

2026년 9월 13일 확인 기준 Windows x64 실행기는 7/70, Linux x64／ARM64는 각각 1/64, 1/62 탐지입니다. Linux의 Microsoft 판정은 `Trojan:Script/Wacatac.C!ml`입니다. 세 파일 모두 현재 소스로 동일하게 재현했으나 안전성이나 오탐을 확정하는 증거는 아닙니다. 이번 macOS 검사에서는 경고가 보고되지 않았지만 전체 보안 검증을 뜻하지는 않습니다.

인코딩과 디코딩 테스트를 분리하고 1초 길게 누르는 심층 테스트를 추가했습니다. Windows에 FFmpeg LGPL 기반 H.264／HEVC／AV1 소프트웨어 디코딩과 네이티브 형식, D3D11／D3D12 분석을 추가했습니다. AUTO에서만 선택 가능한 전송 전략은 균형(기본값), 낮은 지연 시간, 대역폭 절약입니다. 하드웨어와 실측 크기 증거를 우선하고, 실패한 인코더 다음 후보를 시도합니다. FPS·비트레이트·GOP 설정은 유지합니다. Windows DLL 누락과 시작 오류 표시, 분석 화면 및 전송 처리를 개선했습니다.

macOS ARM64, Windows x64／ARM64, Linux x64／ARM64 CLI, 실험용 WinPE x64를 제공하며 Android는 제외합니다. 제품 테스트와 Smoke, 교차 컴파일, DLL 검사, macOS 서명·공증을 확인했습니다. Windows 실제 장치의 드라이버, 연결 성능 및 SYSTEM 서비스 제한은 추가 검증이 필요합니다. 네이티브 HEVC 인코딩 성공은 정식 RGBA 스트림의 HEVC 인코더 지원을 뜻하지 않습니다.

## 檢測報告／Detection reports

- [Windows x64 launcher](https://www.virustotal.com/gui/file/043b5952ba4c2a50edb6d320a0d5145713ae2e0ee1ae8aedac3f4db19c023871)
- [Linux x64 client](https://www.virustotal.com/gui/file/a4de9638adff156da70e111138dbd8fe96b5ddf891455a2c1415f20770c63502)
- [Linux ARM64 client](https://www.virustotal.com/gui/file/d5dd60197c8d038b34d775f2ef7669ebb522542d2e7686effb614af967d10784)

- [Go empty project / 空專案（掃描快照 9/70）](https://www.virustotal.com/gui/file/fb906e6611161faafc695ada5042ae82696b88dba3cd1309843d1f42adfe084f)
- [Go 官方 FAQ：Antivirus detections](https://go.dev/doc/faq#virus)
