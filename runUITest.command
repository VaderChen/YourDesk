#!/bin/bash
set -Eeuo pipefail

# 編譯並開啟本機 Client 桌面管理介面。
ROOT="$(cd "$(dirname "$0")" && pwd)"
export PATH="$PATH:/opt/homebrew/bin:/usr/local/bin"
finish() {
  local result=$?
  trap - EXIT
  if [[ "$result" -ne 0 ]]; then
    echo "啟動失敗（結束碼：$result），請查看上方訊息。" >&2
    if [[ -t 0 && "$result" -ne 130 && "$result" -ne 143 ]]; then
      read -r -p "按 Enter 關閉視窗……" _ || true
    fi
  fi
  exit "$result"
}
trap finish EXIT
command -v go >/dev/null 2>&1 || { echo "找不到 Go 工具鏈。" >&2; exit 1; }
cd "$ROOT"
BUILD_VERSION="1.$(date +%y.%m%d) build $(date +%H%M)"

mkdir -p bin
echo "編譯 YourDesk Client 與 Viewer……"
CGO_ENABLED=1 go build -ldflags "-X 'yourdesk/internal/clientui.Version=$BUILD_VERSION'" -o bin/yourdesk-client ./cmd/client
go build -ldflags "-X 'yourdesk/internal/clientui.Version=$BUILD_VERSION'" -o bin/yourdesk-remote ./cmd/remote
"$ROOT/scripts/sign-local.sh" "$ROOT/bin/yourdesk-client" "$ROOT/bin/yourdesk-remote"
echo "開啟 Client 桌面視窗……"
./bin/yourdesk-client -ui -signal "${YOURDESK_SIGNAL_URL:-wss://${YOURDESK_SIGNAL_HOST:-desktop.mars-cloud.com}:${YOURDESK_SIGNAL_PORT:-8080}/ws}" -room "${YOURDESK_ROOM:-}" "$@"
