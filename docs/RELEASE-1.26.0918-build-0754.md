## 繁體中文

修正遠端 Windows 畫面持續串流，但鍵盤與滑鼠無法操作的問題。Windows Host 綁定輸入桌面時補齊 `DESKTOP_JOURNALPLAYBACK` 權限，避免 `SendInput` 遭拒絕存取。已確認更新後測試有效；這項修正需更新被控 Windows Host。

- 移除本次排查專用的 `viewer-input` 定期日誌及輸入計數；保留既有封包分析與正常錯誤回報。
- Windows 輸入使用固定 OS 執行緒並跟隨目前輸入桌面。遠端桌面連線期間防止自動休眠與關閉螢幕，斷線後解除；不取消系統鎖定或螢幕保護程式。
- Mac 游標改以 AppKit 取樣，工具列、遠端操作與裁切共用座標，改善觸控板位置未更新的情況。
- 補強心跳與控制工作者停滯偵測，以及 Windows GPU 讀回等待防護。新增連線計時、預設關閉的 15 分鐘閒置自動關閉，以及預設關閉的實驗性自動重連。自動重連不保證檔案續傳。

驗證範圍：確認本次鍵鼠修正有效；Windows 10 API 比較亦確認原桌面權限遭拒、補齊後成功。

套件：macOS arm64 簽章／公證 DMG、Windows x64／arm64 安裝版、Windows x64 免安裝 ZIP、Linux x64／arm64 CLI Host、WinPE x64 實驗版。Android 不納入本次發行。

## English

Fixes an issue where video from a remote Windows computer continued streaming, but keyboard and mouse control did not work. The Windows Host now requests `DESKTOP_JOURNALPLAYBACK` access when binding to the input desktop, preventing access-denied failures in `SendInput`. Testing confirmed the fix after updating. Update the controlled Windows Host to apply this fix.

- Removes the temporary `viewer-input` periodic logs and input counters added for this investigation. Existing Packet Analysis and normal error reporting remain available.
- Windows input runs on a dedicated OS thread and follows the current input desktop. Automatic sleep and display power-off are prevented during a remote desktop connection; this requirement is released on disconnect. System locking and screen savers remain enabled.
- Mac cursor sampling now uses AppKit, with shared coordinates for the toolbar, remote control and cropping, improving cases where trackpad position updates were missed.
- Improves heartbeat and control-worker stall detection, and safeguards Windows GPU readback waits. Adds a connection timer, an option to close after 15 minutes of inactivity, and experimental automatic reconnection. Both options are disabled by default. Automatic reconnection does not guarantee file-transfer resumption.

Validation: testing confirmed the keyboard and mouse fix. Windows 10 API comparisons also confirmed that the original desktop access rights caused access-denied failures and that adding the missing right resolved them.

Packages: signed/notarized macOS arm64 DMG; Windows x64/arm64 installers; Windows x64 portable ZIP; Linux x64/arm64 CLI Host; experimental WinPE x64. Android is not included in this release.

## 日本語

リモート Windows の映像は配信され続けているのに、キーボードとマウスで操作できない問題を修正しました。Windows Host が入力デスクトップにスレッドを関連付ける際に `DESKTOP_JOURNALPLAYBACK` アクセス権を追加し、`SendInput` のアクセス拒否を防ぎます。更新後のテストで修正の有効性を確認しました。適用には操作対象の Windows Host を更新してください。

- 今回の調査用に追加した `viewer-input` の定期ログと入力カウンターを削除しました。既存のパケット解析と通常のエラー報告は維持します。
- Windows の入力処理は専用の OS スレッドで実行し、現在の入力デスクトップに追従します。リモートデスクトップ接続中は自動スリープとディスプレイの自動電源オフを抑止し、切断後に解除します。システムのロックやスクリーンセーバーは無効にしません。
- Mac のカーソル位置を AppKit で取得し、ツールバー、リモート操作、切り抜きで座標を共通化しました。トラックパッドの位置更新が反映されないケースを改善します。
- ハートビートと制御ワーカーの停止検知、および Windows GPU の読み戻し待機に対する保護を強化しました。接続時間の表示、15 分間操作がない場合の自動終了、実験的な自動再接続を追加しました。自動終了と自動再接続は初期設定では無効です。自動再接続はファイル転送の再開を保証しません。

検証：今回のキーボード・マウス修正が有効であることを確認しました。Windows 10 の API 比較でも、従来のデスクトップアクセス権では拒否され、不足する権限の追加後に成功することを確認しました。

配布パッケージ：署名・公証済み macOS arm64 DMG、Windows x64／arm64 インストーラー、Windows x64 ポータブル ZIP、Linux x64／arm64 CLI Host、実験版 WinPE x64。Android は今回のリリースに含まれません。

## 한국어

원격 Windows 화면은 계속 스트리밍되지만 키보드와 마우스로 조작할 수 없는 문제를 수정했습니다. Windows Host가 입력 데스크톱에 스레드를 연결할 때 `DESKTOP_JOURNALPLAYBACK` 접근 권한을 추가하여 `SendInput`의 접근 거부를 방지합니다. 업데이트 후 테스트에서 수정 효과를 확인했습니다. 이 수정 사항을 적용하려면 제어 대상 Windows Host를 업데이트하세요.

- 이번 조사에 추가했던 `viewer-input` 주기 로그와 입력 카운터를 제거했습니다. 기존 패킷 분석과 일반 오류 보고는 유지합니다.
- Windows 입력은 전용 OS 스레드에서 처리하며 현재 입력 데스크톱을 따릅니다. 원격 데스크톱 연결 중에는 자동 절전과 디스플레이 자동 꺼짐을 방지하고, 연결을 끊으면 해제합니다. 시스템 잠금이나 화면 보호기는 비활성화하지 않습니다.
- Mac 커서 위치를 AppKit으로 수집하고 도구 모음, 원격 조작, 자르기에 공통 좌표를 사용하여 트랙패드 위치 갱신이 반영되지 않는 경우를 개선했습니다.
- 하트비트 및 제어 작업자의 정지 감지와 Windows GPU 읽기 대기 보호를 강화했습니다. 연결 시간 표시, 15분간 조작이 없을 때 자동 종료하는 옵션, 실험적 자동 재연결을 추가했습니다. 자동 종료와 자동 재연결은 기본적으로 꺼져 있습니다. 자동 재연결은 파일 전송 재개를 보장하지 않습니다.

검증: 이번 키보드·마우스 수정의 효과를 확인했습니다. Windows 10 API 비교에서도 기존 데스크톱 접근 권한으로는 거부되고, 누락된 권한을 추가하면 성공함을 확인했습니다.

패키지: 서명·공증된 macOS arm64 DMG, Windows x64／arm64 설치 프로그램, Windows x64 포터블 ZIP, Linux x64／arm64 CLI Host, 실험 버전 WinPE x64. Android는 이번 릴리스에 포함되지 않습니다.
