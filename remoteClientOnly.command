#!/bin/bash
set -Eeuo pipefail
umask 077

# 被控端專用：重新編譯 Client，連接遠端配對服務並保留診斷日誌。
ROOT="$(cd "$(dirname "$0")" && pwd)"
BIN="$ROOT/bin"
RUN_DIR="$ROOT/.remote-run"
SIGNAL_URL="${YOURDESK_SIGNAL_URL:-wss://${YOURDESK_SIGNAL_HOST:-desktop.mars-cloud.com}:${YOURDESK_SIGNAL_PORT:-8080}/ws}"
ROOM="${YOURDESK_ROOM:-}"
CLIENT_ARGS=()

usage() {
  cat <<'EOF'
用法：
  ./remoteClientOnly.command
	./remoteClientOnly.command -room <ROOM>
  ./remoteClientOnly.command -signal <wss://主機:連接埠/ws> -codec software-jpeg

只啟動被控端 Client，連接 MarsCloud 配對服務。
ROOM 預設使用本機硬體 UID，連線密碼首次自動產生高強度編碼，之後沿用本機保存值。
可另外指定 -display、-fps、-quality、-codec。
也支援 YOURDESK_ROOM、YOURDESK_SIGNAL_URL、
YOURDESK_SIGNAL_HOST 與 YOURDESK_SIGNAL_PORT 環境變數。
EOF
}

finish() {
  local result=$?
  trap - EXIT
  if [[ "$result" -ne 0 ]]; then
    echo "Client 啟動或執行失敗（結束碼：$result）。請保留上方訊息與日誌。" >&2
    if [[ -t 0 && "$result" -ne 130 && "$result" -ne 143 ]]; then
      read -r -p "按 Enter 關閉視窗……" _ || true
    fi
  fi
  exit "$result"
}
trap finish EXIT

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help) usage; exit 0 ;;
	-room|-signal|-display|-fps|-quality|-codec)
      if [[ $# -lt 2 || -z "$2" ]]; then
        echo "$1 缺少參數值。" >&2
        exit 2
      fi
      case "$1" in
        -room) ROOM="$2" ;;
        -signal) SIGNAL_URL="$2" ;;
        *) CLIENT_ARGS+=("$1" "$2") ;;
      esac
      shift 2
      ;;
    *) echo "未知參數：$1" >&2; usage >&2; exit 2 ;;
  esac
done

# Finder 啟動 Terminal 時，PATH 不一定包含 Homebrew 的 Go。
export PATH="$PATH:/opt/homebrew/bin:/usr/local/bin"
if ! command -v go >/dev/null 2>&1; then
  echo "找不到 Go，請先安裝專案要求的 Go 工具鏈。" >&2
  exit 1
fi

mkdir -p "$BIN" "$RUN_DIR"
CLIENT_LOG="$(mktemp "$RUN_DIR/client-only-$(date '+%Y%m%d-%H%M%S').log.XXXXXX")"
echo "日誌：$CLIENT_LOG"

run_client() {
  echo "重新編譯 YourDesk Client……"
  (cd "$ROOT" && go build -o "$BIN/yourdesk-client" ./cmd/client) || return $?
  "$ROOT/scripts/sign-local.sh" "$BIN/yourdesk-client" || return $?
  if [[ -z "$ROOM" ]]; then
    ROOM="$("$BIN/yourdesk-client" -print-uid)" || return $?
  fi
  echo "ROOM：$ROOM"
  echo "配對服務：$SIGNAL_URL"
  echo "macOS 請允許螢幕錄製與輔助使用；變更權限後重新執行。"
  echo "等待 Viewer 連線；按 Ctrl-C 結束 Client。"
	"$BIN/yourdesk-client" -signal "$SIGNAL_URL" -room "$ROOM" ${CLIENT_ARGS[@]+"${CLIENT_ARGS[@]}"}
}

run_client 2>&1 | tee -a "$CLIENT_LOG"
