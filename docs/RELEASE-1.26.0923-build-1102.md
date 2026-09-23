## 繁體中文

- Mac 檔案傳輸新增「磁碟」入口，可瀏覽已掛載外接硬碟，重新整理可更新插拔狀態。
- 顯示區域新增遠端聲音喇叭快捷開關；修正設定同步時可能覆蓋聲音開關的問題。
- 檔案傳輸視窗開啟或再次開啟時自動移至前景，保留既有傳輸佇列。

請一併更新控制端與遠端 Mac，以使用完整磁碟清單與名稱顯示。喇叭快捷開關與進階設定共用偏好，會套用至目前開啟的顯示區域；遠端聲音仍為實驗性功能，預設關閉。磁碟根目錄不可刪除，一般隱藏檔過濾與檔案存取權限保持不變。

已完成相關 Go 測試、53 項檔案介面回歸、聲音設定及標題列瀏覽器 Smoke、Mac 原生檔案視窗 Smoke，並從本機掛載表確認外接磁碟列舉。Windows／Linux 套件建置不代表已完成實機音訊或外接磁碟傳輸驗證。

## English

- Mac file transfer adds a Disks entry for mounted external drives; refresh updates connected drives.
- The display toolbar adds a remote-audio toggle; settings changes no longer overwrite an unsynchronized audio toggle.
- Opening or reopening a file-transfer window brings it to the foreground while preserving its transfer queue.

Update both the controlling computer and remote Mac for the full disk list and readable disk names. The speaker toggle shares the Advanced Settings preference and applies to all open display windows. Remote audio remains experimental and off by default. Disk roots cannot be deleted; hidden-file filtering and file permissions remain in effect.

Validation includes relevant Go tests, 53 file-UI regressions, browser smoke tests for audio settings and the title bar, native Mac file-window smoke tests, and external-drive enumeration from the local mount table. Windows/Linux package builds do not constitute on-device audio or external-drive transfer validation.

## 日本語

- Mac のファイル転送に「ディスク」を追加し、マウント済みの外付けディスクを参照できます。更新で接続状態を反映します。
- 表示領域にリモート音声の切り替えボタンを追加し、設定の保存が未同期の音声状態を上書きする問題を修正しました。
- ファイル転送ウィンドウを開くと前面に表示します。再表示時も転送キューを保持します。

ディスク一覧と名前表示を利用するには、操作側とリモート Mac の両方を更新してください。音声ボタンは詳細設定と共通で、開いているすべての表示ウィンドウに適用されます。リモート音声は引き続き実験的機能で、初期設定はオフです。ディスクのルートは削除できず、隠しファイルの除外とアクセス権限は維持されます。

関連 Go テスト、ファイル UI の回帰 53 項目、音声設定・タイトルバーのブラウザー Smoke、Mac のネイティブ転送ウィンドウ Smoke、実際のマウント表による外付けディスク列挙を確認しました。Windows／Linux のビルドは、実機での音声や外付けディスク転送の検証を意味しません。

## 한국어

- Mac 파일 전송에 디스크 항목을 추가하여 마운트된 외장 드라이브를 탐색합니다. 새로 고침으로 연결 상태를 갱신합니다.
- 표시 영역에 원격 오디오 전환 버튼을 추가하고, 설정 저장이 아직 동기화되지 않은 오디오 상태를 덮어쓰는 문제를 수정했습니다.
- 파일 전송 창을 열거나 다시 열면 앞으로 가져오며 기존 전송 대기열은 유지합니다.

전체 디스크 목록과 이름 표시를 사용하려면 제어 컴퓨터와 원격 Mac을 모두 업데이트하세요. 스피커 버튼은 고급 설정과 같은 환경설정을 사용하며 열린 모든 표시 창에 적용됩니다. 원격 오디오는 계속 실험적 기능이며 기본값은 꺼짐입니다. 디스크 루트 삭제는 차단되며 숨김 파일 필터와 접근 권한은 유지됩니다.

관련 Go 테스트, 파일 UI 회귀 53개, 오디오 설정과 제목 표시줄 브라우저 Smoke, Mac 네이티브 파일 창 Smoke 및 실제 마운트 테이블의 외장 디스크 열거를 확인했습니다. Windows/Linux 패키지 빌드는 실제 장치의 오디오 또는 외장 디스크 전송 검증을 의미하지 않습니다.
