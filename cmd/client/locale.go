package main

import (
	"os"
	"strings"
)

// POSIX 語系優先順序；C/POSIX 及不支援語系使用英文。
var cliLanguage = detectCLILanguage()

func detectCLILanguage() string {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			value = strings.ToLower(strings.ReplaceAll(value, "-", "_"))
			value = strings.Split(strings.Split(value, ".")[0], "@")[0]
			switch strings.Split(value, "_")[0] {
			case "zh":
				return "zh"
			case "ja":
				return "ja"
			case "ko":
				return "ko"
			default:
				return "en"
			}
		}
	}
	return "en"
}

func cliText(text string) string {
	index := map[string]int{"zh": 0, "ja": 1, "ko": 2}
	if i, ok := index[cliLanguage]; ok {
		if translated, found := cliMessages[text]; found {
			return translated[i]
		}
	}
	return text
}

var cliMessages = map[string][3]string{
	"No desktop session detected; terminal connections only":                                            {"未偵測到桌面環境，僅提供命令列連線", "デスクトップ環境がないため、ターミナル接続のみ提供します", "데스크톱 환경이 없어 터미널 연결만 제공합니다"},
	"Connection password loaded and validated. Starting Host.":                                          {"連線密碼已驗證，正在啟動 Host。", "パスワードを確認しました。Host を起動します。", "암호를 확인했습니다. Host를 시작합니다."},
	"Connecting to the signaling server...":                                                             {"正在連線配對伺服器…", "接続サーバーに接続しています…", "연결 서버에 연결하는 중…"},
	"Signaling channel established. Waiting for a remote connection.":                                   {"配對通道已建立，正在等待遠端連線。", "接続チャネルを確立しました。リモート接続を待っています。", "연결 채널이 열렸습니다. 원격 연결을 기다립니다."},
	"TLS server check passed":                                                                           {"TLS 伺服器驗證成功", "TLS サーバーの確認に成功しました", "TLS 서버 확인 성공"},
	"Interactive terminal is unavailable for this account. Run as a regular user without root or sudo.": {"目前帳號無法使用互動命令列，請以一般使用者執行，不要使用 root 或 sudo。", "このアカウントでは対話ターミナルを利用できません。root や sudo を使わず一般ユーザーで実行してください。", "이 계정은 대화형 터미널을 사용할 수 없습니다. root나 sudo 없이 일반 사용자로 실행하세요."},
	"A Host is already running for this device. Waiting for it to exit.":                                {"此裝置已有 Host 執行，正在等待它結束。", "この装置の Host は実行中です。終了を待っています。", "이 장치의 Host가 이미 실행 중입니다. 종료를 기다립니다."},
	"Direct IP connection ended":                                                                        {"IP 直連已結束", "IP 直接接続が終了しました", "IP 직접 연결 종료"},
	"Could not start direct IP listener":                                                                {"無法啟動 IP 直連", "IP 直接接続を開始できません", "IP 직접 연결을 시작할 수 없습니다"},
	"Signaling connection ended; retrying shortly":                                                      {"配對連線已結束，稍後重試", "接続チャネルが終了しました。まもなく再試行します", "연결 채널이 종료되었습니다. 잠시 후 다시 시도합니다"},
	"Could not read connection password JSON from standard input":                                       {"無法從標準輸入讀取密碼 JSON", "標準入力からパスワード JSON を読み取れません", "표준 입력에서 암호 JSON을 읽을 수 없습니다"},
	"Waiting for password: enter one JSON line and press Enter, e.g. {\"secret\":\"YOUR_PASSWORD\"}.":   {"等待密碼：請輸入一行 JSON 並按 Enter，例如 {\"secret\":\"YOUR_PASSWORD\"}。", "パスワード待ち：JSON を 1 行入力して Enter を押してください。例：{\"secret\":\"YOUR_PASSWORD\"}。", "암호 대기: JSON 한 줄을 입력하고 Enter를 누르세요. 예: {\"secret\":\"YOUR_PASSWORD\"}."},
	"Unexpected positional arguments; use -secret PASSWORD to set the connection password":              {"不接受額外參數；請用 -secret 密碼 指定連線密碼", "追加の引数は使えません。-secret PASSWORD でパスワードを指定してください", "추가 인수는 사용할 수 없습니다. -secret PASSWORD로 암호를 지정하세요"},
	"-secret cannot be combined with -secret-stdin, -parent-stdin or -prelogin-host":                    {"-secret 不可與 -secret-stdin、-parent-stdin 或 -prelogin-host 同時使用", "-secret は -secret-stdin、-parent-stdin、-prelogin-host と併用できません", "-secret은 -secret-stdin, -parent-stdin, -prelogin-host와 함께 사용할 수 없습니다"},
	"-secret is only supported for CLI Hosts; change the password in Settings for desktop mode":         {"-secret 僅用於 CLI Host；桌面模式請在設定中修改密碼", "-secret は CLI Host 専用です。デスクトップでは設定で変更してください", "-secret은 CLI Host 전용입니다. 데스크톱에서는 설정에서 변경하세요"},
	"parent-stdin requires secret-stdin":                                                                {"parent-stdin 必須搭配 secret-stdin", "parent-stdin には secret-stdin が必要です", "parent-stdin에는 secret-stdin이 필요합니다"},
	"Service Host must be started through a private parent pipe":                                        {"服務 Host 必須由私有父管線啟動", "サービス Host は専用の親パイプから起動してください", "서비스 Host는 전용 부모 파이프로 시작해야 합니다"},
	"YourDesk pre-login service:":                                                                       {"YourDesk 未登入服務：", "YourDesk ログイン前サービス：", "YourDesk 로그인 전 서비스:"},
	"rendezvous WebSocket URL":                                                                          {"配對伺服器 WebSocket 網址", "接続サーバーの WebSocket URL", "연결 서버 WebSocket URL"},
	"pairing room; empty means use hardware UID":                                                        {"裝置 ID；空白使用硬體 ID", "装置 ID。空欄はハードウェア ID", "장치 ID. 비어 있으면 하드웨어 ID 사용"},
	"Host authorized by the system service":                                                             {"由系統服務授權的 Host", "システムサービス認証済み Host", "시스템 서비스가 승인한 Host"},
	"Stop the Host when the parent app closes its input pipe":                                           {"父程式關閉輸入管線時停止 Host", "親アプリが入力パイプを閉じたら Host を停止", "부모 앱이 입력 파이프를 닫으면 Host 중지"},
	"Connection password for this CLI Host run; does not change the saved password":                     {"本次 CLI 連線密碼；不修改已儲存密碼", "今回の CLI 接続パスワード。保存済みの値は変更しません", "이번 CLI 연결 암호. 저장된 암호는 변경하지 않음"},
	"Read connection password JSON from standard input for automation":                                  {"從標準輸入讀取密碼 JSON，供自動化使用", "自動化用に標準入力からパスワード JSON を読み取り", "자동화를 위해 표준 입력에서 암호 JSON 읽기"},
	"Print the saved connection password (generate on first use) and exit":                              {"顯示已儲存密碼（首次自動產生）後結束", "保存済みパスワードを表示（初回は生成）して終了", "저장된 암호 표시(최초 실행 시 생성) 후 종료"},
	"print hardware-derived device UID and exit":                                                        {"顯示硬體裝置 ID 後結束", "ハードウェア装置 ID を表示して終了", "하드웨어 장치 ID 표시 후 종료"},
	"Open the Client desktop window":                                                                    {"開啟 Client 桌面視窗", "Client のデスクトップ画面を開く", "Client 데스크톱 창 열기"},
	"Listen address for direct IP connections over TLS; opt-in":                                         {"IP 直連 TLS 監聽位址；須明確啟用", "IP 直接接続の TLS 待受アドレス。有効化が必要", "IP 직접 연결 TLS 수신 주소. 명시적으로 활성화"},
	"Check the signaling server over TLS and exit":                                                      {"檢查配對伺服器 TLS 後結束", "接続サーバーの TLS を確認して終了", "연결 서버 TLS 확인 후 종료"},
	"Transport mode: empty for native UDP, tailcat for experimental Tailcat":                            {"傳輸模式：空白使用原生 UDP，tailcat 為實驗性功能", "転送方式：空欄はネイティブ UDP、tailcat は実験機能", "전송 모드: 비어 있으면 기본 UDP, tailcat은 실험 기능"},
	"display index":       {"螢幕索引", "画面番号", "화면 번호"},
	"maximum capture FPS": {"最高擷取 FPS", "最大キャプチャ FPS", "최대 캡처 FPS"},
	"JPEG quality 1-100":  {"JPEG 品質 1–100", "JPEG 品質 1–100", "JPEG 품질 1–100"},
	"codec mode: auto, hardware-h264, hardware-hevc, software-jpeg": {"編碼模式：auto、hardware-h264、hardware-hevc、software-jpeg", "コーデック：auto、hardware-h264、hardware-hevc、software-jpeg", "코덱: auto, hardware-h264, hardware-hevc, software-jpeg"},
}
