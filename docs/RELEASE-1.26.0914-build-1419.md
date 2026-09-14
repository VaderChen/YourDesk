## 繁體中文

- 新增 Viewer 裁切串流：預設裁切框置中、寬高各半並對齊 8 像素。拖曳框內移動、邊角調整，再按裁切套用；Host 只串流選取區域。按復原會先顯示 WebView 確認對話框，確認後恢復全畫面。Host 須支援區域串流。
- 修正 macOS 調整裁切框時背景變黑；裁切與 Retina／縮放畫面的滑鼠座標沿用同一區域換算。
- 修正 Shift／Ctrl 等修飾鍵與滑鼠事件的送出順序，macOS 點擊、拖曳及滾輪套用修飾鍵。Viewer 與 macOS Host 都需更新。
- 本機 ID 旁新增站台 QR Code，可分享名稱、ID 與伺服器位址，不包含密碼。此版僅提供桌面端產生功能，不包含手機 App 更新。
- 工具列僅保留 FPS，泡泡顯示畫面增強、策略、演算法、尺寸及碼率；增強生效時 FPS 呈綠色。TX／RX 上下排列，分別使用暗紅／暗綠；設備 ID 縮小。修正 Windows 圖示靠右排列，裁切旁加入分隔線。
- 站台已連線時圖示顯示綠色，建立連線中為琥珀色；不因 Presence 查詢失敗而把已連線設備顯示為灰色。

驗證：使用者回報 Windows 裁切可用；本機已完成編譯與相關 Smoke 檢查。最新對話框、工具列調整及跨機修飾鍵操作仍需實機持續驗證，不以編譯成功代替所有平台功能驗證。沿用前版串流穩定修正；先前停滯回報僅見於 macOS 對 macOS，仍不宣稱所有斷線均已完全解決。本版未啟用背景自動重連／續傳。

套件：macOS arm64 簽章／公證 DMG、Windows x64／arm64 安裝版、Windows x64 免安裝 ZIP、Linux x64／arm64 CLI Host、WinPE x64 實驗版。Android 不納入本次發行。

部分防毒軟體曾提出告警；空 Go 專案也曾出現偵測，這不是本版套件掃描結果，亦不能證明所有警告都是誤判。作者會持續朝清除告警的方向努力。[空專案 VirusTotal 報告](https://www.virustotal.com/gui/file/fb906e6611161faafc695ada5042ae82696b88dba3cd1309843d1f42adfe084f) · [Go 官方 FAQ](https://go.dev/doc/faq#virus)。

## English

- Adds Viewer region streaming with a centered half-width/half-height crop aligned to 8 pixels. Drag the box or handles, then press Crop to apply. Restore asks for confirmation in a WebView before returning to the full screen. The Host must support region streaming.
- Fixes the black background while adjusting a crop on macOS; pointer mapping accounts for the selected region, Retina and display scaling.
- Preserves modifier-key/mouse ordering and applies modifiers to macOS clicks, drags and scrolling. Update both the Viewer and macOS Host.
- Adds desktop site QR codes containing the name, ID and signaling address, without passwords. No mobile app update is included.
- Consolidates enhancement details into the FPS tooltip; FPS turns green when enhancement is active. Stacks dark-red TX and dark-green RX, reduces the ID font, fixes Windows toolbar alignment and adds a crop separator.
- Connected sites stay green even when presence lookup fails; connecting sites use amber.

The user reported working cropping on Windows. Compilation and local smoke checks passed; the latest dialogs, toolbar changes and cross-device modifier input still need real-device validation. Earlier freeze reports were Mac-to-Mac only; prior stability fixes remain, but not every disconnect is confirmed resolved. No enabled background auto-reconnect or transfer resumption.

Packages: signed/notarized macOS arm64 DMG; Windows x64/arm64 installers; Windows x64 portable ZIP; Linux x64/arm64 CLI Host; experimental WinPE x64. No Android release.

An empty Go project also triggered antivirus detections; this is not a scan of these assets or proof that every warning is false. The author continues working to clear alerts. [Empty-project VirusTotal](https://www.virustotal.com/gui/file/fb906e6611161faafc695ada5042ae82696b88dba3cd1309843d1f42adfe084f) · [Official Go FAQ](https://go.dev/doc/faq#virus).

## 日本語

- Viewer に領域ストリーミングを追加。初期枠は中央・幅と高さが各半分で、8 ピクセル単位に調整します。枠やハンドルをドラッグし、再度切り抜きを押して適用します。復元は WebView の確認後に画面全体へ戻ります。Host の領域配信対応が必要です。
- macOS の切り抜き編集中に背景が黒くなる問題を修正。領域、Retina、表示倍率に合わせてマウス座標を変換します。
- 修飾キーとマウスの送信順序を修正し、macOS のクリック、ドラッグ、スクロールに適用します。Viewer と macOS Host の両方を更新してください。
- 名前、ID、サーバー情報を含むパスワードなしのデスクトップ用 QR コードを追加。モバイルアプリ更新は含みません。
- 増強情報を FPS の吹き出しへ統合し、有効時は緑色に表示。TX／RX を上下に配置し暗赤／暗緑に変更。ID の文字を縮小し、Windows の右寄せと切り抜きの区切り線を調整しました。
- 接続済みは緑、接続中は琥珀色。オンライン確認に失敗しても接続済みを灰色にしません。

ユーザーから Windows の切り抜き動作報告があり、コンパイルとローカル Smoke も完了しています。最新 UI と端末間の修飾キー操作は継続実機確認が必要です。従来の停止報告は Mac 間のみで、全切断の解決を断定しません。背景自動再接続・転送再開は有効化していません。

macOS arm64 署名／公証 DMG、Windows x64／arm64 インストーラー、Windows x64 ポータブル、Linux x64／arm64 CLI Host、実験版 WinPE x64 を配布。Android は対象外です。

空 Go プロジェクトにも検出例がありますが、今回の配布物の検査結果や全警告の誤検知の証明ではありません。作者は警告解消に向け改善を続けます。[空プロジェクト VirusTotal](https://www.virustotal.com/gui/file/fb906e6611161faafc695ada5042ae82696b88dba3cd1309843d1f42adfe084f) · [Go 公式 FAQ](https://go.dev/doc/faq#virus)。

## 한국어

- Viewer 영역 스트리밍을 추가했습니다. 기본 자르기 상자는 화면 중앙에 가로·세로 절반 크기로 배치하며 8픽셀 단위로 맞춥니다. 상자와 핸들을 드래그하고 자르기를 다시 눌러 적용합니다. 복원은 WebView 확인 후 전체 화면으로 돌아갑니다. Host의 영역 스트리밍 지원이 필요합니다.
- macOS 자르기 편집 중 검은 배경을 수정했습니다. 영역, Retina 및 화면 배율에 맞춰 마우스 좌표를 변환합니다.
- 보조 키와 마우스의 전송 순서를 수정하고 macOS 클릭·드래그·스크롤에 보조 키를 적용합니다. Viewer와 macOS Host 모두 업데이트해야 합니다.
- 이름, ID, 서버 정보를 담고 비밀번호는 제외하는 데스크톱 QR 코드를 추가했습니다. 모바일 앱 업데이트는 포함하지 않습니다.
- 화면 향상 정보를 FPS 말풍선으로 통합하고 활성화 시 녹색으로 표시합니다. TX／RX를 위아래로 배치하고 어두운 빨강／초록을 적용했습니다. ID 글자를 줄이고 Windows 오른쪽 정렬과 자르기 구분선을 수정했습니다.
- 연결됨은 초록, 연결 중은 호박색이며 온라인 조회 실패로 연결된 기기를 회색으로 표시하지 않습니다.

사용자가 Windows 자르기 동작을 확인했고 컴파일 및 로컬 Smoke 검사를 완료했습니다. 최신 UI와 기기 간 보조 키 입력은 계속 실제 기기 확인이 필요합니다. 이전 멈춤 보고는 Mac 간에서만 발생했으며 모든 연결 끊김이 해결되었다고 단정하지 않습니다. 백그라운드 자동 재연결／전송 재개는 활성화하지 않았습니다.

서명／공증된 macOS arm64 DMG, Windows x64／arm64 설치 프로그램, Windows x64 포터블, Linux x64／arm64 CLI Host, 실험용 WinPE x64를 제공합니다. Android는 제외합니다.

빈 Go 프로젝트도 탐지된 사례가 있지만 이번 배포물 검사 결과나 모든 경고가 오탐이라는 증거는 아닙니다. 개발자는 경고 해소를 위해 계속 개선합니다. [빈 프로젝트 VirusTotal](https://www.virustotal.com/gui/file/fb906e6611161faafc695ada5042ae82696b88dba3cd1309843d1f42adfe084f) · [Go 공식 FAQ](https://go.dev/doc/faq#virus).
