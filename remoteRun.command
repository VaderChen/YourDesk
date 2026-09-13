#!/bin/bash
set -Eeuo pipefail

# Connect to the deployed signaling server. This script does not start a
# local signaling server and does not relay desktop data.
#
# Viewer only (recommended; ROOM defaults to this Mac's hardware UID):
#   ./remoteRun.command
#
# Client-only mode (this Mac waits for another Viewer):
#   ./remoteRun.command --client-only
# Local smoke/client mode:
#   ./remoteRun.command --with-client

ROOT="$(cd "$(dirname "$0")" && pwd)"
BIN="$ROOT/bin"
RUN_DIR="$ROOT/.remote-run"
SIGNAL_HOST="${YOURDESK_SIGNAL_HOST:-desktop.mars-cloud.com}"
SIGNAL_PORT="${YOURDESK_SIGNAL_PORT:-8080}"
SIGNAL_URL="wss://${SIGNAL_HOST}:${SIGNAL_PORT}/ws"
ROOM="${YOURDESK_ROOM:-}"
SECRET=""
WITH_CLIENT=0
CLIENT_ONLY=0

usage() {
  cat <<'EOF'
用法：
  ./remoteRun.command [-room <ROOM>]
  ./remoteRun.command --client-only
  ./remoteRun.command --with-client

預設 ROOM 為本機硬體 UID；可用 -room 或 YOURDESK_ROOM 覆寫。
預設只啟動 Remote Viewer，Client 必須已在被控桌面執行。
--client-only 只啟動本機 Client，連到遠端 Server 等待其他 Viewer。
--with-client 會在本機同時啟動 Client，僅適合測試。
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -room) ROOM="${2:?缺少 ROOM}"; shift 2 ;;
    --client-only) CLIENT_ONLY=1; WITH_CLIENT=1; shift ;;
    --with-client) WITH_CLIENT=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "未知參數：$1" >&2; usage >&2; exit 2 ;;
  esac
done

mkdir -p "$BIN" "$RUN_DIR"
echo "重新 BUILD YourDesk Client / Remote..."
(cd "$ROOT" && python3 scripts/ffmpeg.py darwin/arm64 go build -o "$BIN/yourdesk-client" ./cmd/client && \
  python3 scripts/ffmpeg.py darwin/arm64 go build -o "$BIN/yourdesk-remote" ./cmd/remote)
"$ROOT/scripts/sign-local.sh" "$BIN/"*.dylib "$BIN/yourdesk-client" "$BIN/yourdesk-remote"
echo "BUILD 完成。"

# Keep pairing convenient: use the same stable hardware-derived UID as the
# Client unless the caller explicitly supplied YOURDESK_ROOM or -room.
if [[ -z "$ROOM" ]]; then
  ROOM="$("$BIN/yourdesk-client" -print-uid)"
  echo "預設 ROOM（本機 UID）：$ROOM"
fi

if ! "$BIN/yourdesk-client" -check-signal -signal "$SIGNAL_URL"; then
  echo "無法連線 signaling server：$SIGNAL_HOST:$SIGNAL_PORT" >&2
  exit 1
fi

if [[ "$WITH_CLIENT" == "1" ]]; then
  if [[ -z "$ROOM" ]]; then ROOM="$("$BIN/yourdesk-client" -print-uid)"; fi
  if [[ -z "$SECRET" ]]; then SECRET="$("$BIN/yourdesk-client" -print-secret)"; fi
else
  [[ -n "$ROOM" ]] || { echo "請提供 ROOM" >&2; exit 2; }
	read -r -s -p "請輸入遠端 Client 顯示的連線密碼：" SECRET
	echo
	[[ -n "$SECRET" ]] || { echo "未提供連線密碼" >&2; exit 2; }
fi

secret_json() {
	local value="$1"
	value="${value//\\/\\\\}"
	value="${value//\"/\\\"}"
	value="${value//$'\n'/\\n}"
	value="${value//$'\r'/\\r}"
	value="${value//$'\t'/\\t}"
	printf '{"secret":"%s"}\n' "$value"
}

CLIENT_LOG="$RUN_DIR/client.log"
REMOTE_LOG="$RUN_DIR/remote.log"
CLIENT_PID=""
REMOTE_PID=""
cleanup() {
  trap - EXIT INT TERM
  [[ -n "$REMOTE_PID" ]] && kill "$REMOTE_PID" 2>/dev/null || true
  [[ -n "$CLIENT_PID" ]] && kill "$CLIENT_PID" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

if [[ "$WITH_CLIENT" == "1" ]]; then
  echo "ROOM: $ROOM"
  "$BIN/yourdesk-client" -signal "$SIGNAL_URL" -room "$ROOM" >"$CLIENT_LOG" 2>&1 &
  CLIENT_PID=$!
  sleep 0.5
  kill -0 "$CLIENT_PID" 2>/dev/null || { cat "$CLIENT_LOG" >&2; exit 1; }
fi

if [[ "$CLIENT_ONLY" == "1" ]]; then
  echo "啟動本機 Client（遠端 Server，等待 Viewer）：$SIGNAL_URL"
  echo "ROOM: $ROOM"
  "$BIN/yourdesk-client" -signal "$SIGNAL_URL" -room "$ROOM" >"$CLIENT_LOG" 2>&1 &
  CLIENT_PID=$!
  sleep 0.5
  kill -0 "$CLIENT_PID" 2>/dev/null || { cat "$CLIENT_LOG" >&2; exit 1; }
  echo "Client 已等待連線。Logs: $CLIENT_LOG"
  wait "$CLIENT_PID"
fi

echo "啟動 Remote：$SIGNAL_URL"
secret_json "$SECRET" | "$BIN/yourdesk-remote" -signal "$SIGNAL_URL" -room "$ROOM" -secret-stdin >"$REMOTE_LOG" 2>&1 &
REMOTE_PID=$!

echo "YourDesk Remote 已啟動。"
echo "Logs: $RUN_DIR"
echo "按 Ctrl-C 結束。"
wait "$REMOTE_PID"
