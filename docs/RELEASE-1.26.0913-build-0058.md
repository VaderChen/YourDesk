# YourDesk 1.26.0913 build 0058

## 繁體中文

> **防毒偵測警語：本程式具備 NAT／防火牆穿透連線功能。目前部分防毒軟體會將部分版本判定為含有病毒，原因正在調查與處理中；尚未確認是否由穿透功能觸發，也尚未取得防毒廠商的誤報確認。請自行評估後下載使用，勿為此關閉防毒保護。**

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

> **Antivirus notice: YourDesk supports NAT/firewall traversal. Some antivirus products currently flag some builds as containing malware. We are investigating and addressing these detections. Their connection to traversal has not been established, and vendors have not confirmed false positives. Assess the risk before downloading; do not disable antivirus protection.**

Added a Windows x64 portable ZIP: extract all files and run `YourDesk.exe`. WebView2 Runtime is required. It contains the same binaries as this release’s installer, with required DLLs and licenses.

As checked on September 13, 2026, the Windows x64 launcher has 7/70 detections. Linux x64 and ARM64 clients have 1/64 and 1/62, respectively, with Microsoft reporting `Trojan:Script/Wacatac.C!ml`. All three binaries reproduce byte-for-byte from the current source. This establishes reproducibility, not safety or a confirmed false positive. No macOS alert was reported in this round; that is not a full security assessment.

This release separates encoding and decoding tests, adds a one-second hold to start deep testing, and brings FFmpeg LGPL H.264/HEVC/AV1 software decoding to Windows. Native Windows format and D3D11/D3D12 analysis is expanded. Transmission strategies offer Balanced (default), Low latency and Save bandwidth, enabled only with AUTO encoding. Hardware and measured-size evidence take priority; a failed encoder no longer disables every hardware candidate. Existing FPS, bitrate and GOP settings are preserved. Windows DLL packaging and startup error reporting are fixed, with analysis layout and streaming pipeline improvements.

The release includes macOS ARM64, Windows x64/ARM64 installers, Linux x64/ARM64 CLI ZIPs and an experimental WinPE x64 ZIP. Android is excluded. Product tests and smoke checks passed, Windows builds and DLL dependencies were checked, and macOS App/DMG signing and notarization passed. Windows driver behavior, end-to-end performance and SYSTEM service restrictions still require hardware validation. Successful native HEVC encoding tests do not imply a production Windows RGBA HEVC encoder.

## 日本語

> **ウイルス対策ソフトに関する注意：YourDesk は NAT／ファイアウォール越えの接続に対応しています。現在、一部のバージョンが一部のウイルス対策ソフトでマルウェアと判定されており、原因を調査・対応中です。接続機能との因果関係や誤検知は未確認です。リスクをご判断のうえダウンロードし、ウイルス対策を無効にしないでください。**

Windows x64 ポータブル ZIP を追加しました。全て展開して `YourDesk.exe` を実行してください。WebView2 Runtime が必要です。同版インストーラーと同じ実行ファイル、必要な DLL とライセンスを同梱します。

2026年9月13日の確認では、Windows x64 ランチャーは7/70、Linux x64／ARM64 は1/64、1/62の検出です。Linux の Microsoft 判定は `Trojan:Script/Wacatac.C!ml` です。3ファイルとも現在のソースから同じバイナリを再現できましたが、安全性や誤検知を証明するものではありません。macOS は今回警告の報告がありませんが、完全な安全性検証ではありません。

エンコードとデコードを独立テストに分離し、1秒長押しの詳細テストを追加しました。Windows は FFmpeg LGPL による H.264／HEVC／AV1 ソフトウェアデコード、ネイティブ形式、D3D11／D3D12 分析に対応します。AUTO 時のみ選べる伝送方針はバランス（既定）、低遅延、帯域節約です。ハードウェア能力と実測サイズを優先し、失敗した候補から次の候補へ移行します。FPS、ビットレート、GOP の設定は保持します。Windows の DLL 同梱と起動失敗表示、分析画面と転送処理を改善しました。

macOS ARM64、Windows x64／ARM64、Linux x64／ARM64 CLI、実験版 WinPE x64 を配布し、Android は含みません。製品テスト、Smoke、クロスコンパイル、DLL 検査、macOS 署名・公証を確認しました。Windows 実機のドライバー、接続性能、SYSTEM サービス制限は引き続き検証が必要です。ネイティブ HEVC テスト成功は正式 RGBA ストリームの HEVC エンコード対応を意味しません。

## 한국어

> **백신 경고 안내: YourDesk는 NAT／방화벽 통과 연결 기능을 제공합니다. 현재 일부 백신이 일부 빌드를 악성코드로 판정하여 원인을 조사하고 대응 중입니다. 통과 기능과의 인과관계 및 오탐 여부는 아직 확인되지 않았습니다. 위험을 판단한 후 다운로드하고 백신 보호를 끄지 마세요.**

Windows x64 무설치 ZIP을 추가했습니다. 전체 압축 해제 후 `YourDesk.exe`를 실행하세요. WebView2 Runtime이 필요합니다. 같은 버전 설치 프로그램의 실행 파일과 필수 DLL 및 라이선스를 포함합니다。

2026년 9월 13일 확인 기준 Windows x64 실행기는 7/70, Linux x64／ARM64는 각각 1/64, 1/62 탐지입니다. Linux의 Microsoft 판정은 `Trojan:Script/Wacatac.C!ml`입니다. 세 파일 모두 현재 소스로 동일하게 재현했으나 안전성이나 오탐을 확정하는 증거는 아닙니다. 이번 macOS 검사에서는 경고가 보고되지 않았지만 전체 보안 검증을 뜻하지는 않습니다.

인코딩과 디코딩 테스트를 분리하고 1초 길게 누르는 심층 테스트를 추가했습니다. Windows에 FFmpeg LGPL 기반 H.264／HEVC／AV1 소프트웨어 디코딩과 네이티브 형식, D3D11／D3D12 분석을 추가했습니다. AUTO에서만 선택 가능한 전송 전략은 균형(기본값), 낮은 지연 시간, 대역폭 절약입니다. 하드웨어와 실측 크기 증거를 우선하고, 실패한 인코더 다음 후보를 시도합니다. FPS·비트레이트·GOP 설정은 유지합니다. Windows DLL 누락과 시작 오류 표시, 분석 화면 및 전송 처리를 개선했습니다.

macOS ARM64, Windows x64／ARM64, Linux x64／ARM64 CLI, 실험용 WinPE x64를 제공하며 Android는 제외합니다. 제품 테스트와 Smoke, 교차 컴파일, DLL 검사, macOS 서명·공증을 확인했습니다. Windows 실제 장치의 드라이버, 연결 성능 및 SYSTEM 서비스 제한은 추가 검증이 필요합니다. 네이티브 HEVC 인코딩 성공은 정식 RGBA 스트림의 HEVC 인코더 지원을 뜻하지 않습니다.

## 檢測報告／Detection reports

- [Windows x64 launcher](https://www.virustotal.com/gui/file/043b5952ba4c2a50edb6d320a0d5145713ae2e0ee1ae8aedac3f4db19c023871)
- [Linux x64 client](https://www.virustotal.com/gui/file/a4de9638adff156da70e111138dbd8fe96b5ddf891455a2c1415f20770c63502)
- [Linux ARM64 client](https://www.virustotal.com/gui/file/d5dd60197c8d038b34d775f2ef7669ebb522542d2e7686effb614af967d10784)
