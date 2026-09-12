#!/bin/bash
set -Eeuo pipefail

# 編譯並開啟本機 Client 桌面管理介面。
ROOT="$(cd "$(dirname "$0")" && pwd)"
BUILD_ONLY=0
if [[ "${1:-}" == --build-only ]]; then BUILD_ONLY=1; shift; fi
BUILD_STAGE=""
export PATH="$PATH:/opt/homebrew/bin:/usr/local/bin"
finish() {
  local result=$?
  trap - EXIT
  if [[ -n "$BUILD_STAGE" ]]; then rm -rf "$BUILD_STAGE"; fi
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

mkdir -p bin .local-run
BUILD_STAGE="$(mktemp -d "$ROOT/.local-run/signed-build.XXXXXX")"
echo "編譯 YourDesk Client 與 Viewer……"
CGO_ENABLED=1 go build -ldflags "-X 'yourdesk/internal/clientui.Version=$BUILD_VERSION'" -o "$BUILD_STAGE/yourdesk-client" ./cmd/client
go build -ldflags "-X 'yourdesk/internal/clientui.Version=$BUILD_VERSION'" -o "$BUILD_STAGE/yourdesk-remote" ./cmd/remote
"$ROOT/scripts/sign-local.sh" "$BUILD_STAGE/yourdesk-client" "$BUILD_STAGE/yourdesk-remote"
# 原路徑只會出現已完整簽署的執行檔，避免重建期間留下 ad-hoc 版本。
mv -f "$BUILD_STAGE/yourdesk-client" "$ROOT/bin/yourdesk-client"
mv -f "$BUILD_STAGE/yourdesk-remote" "$ROOT/bin/yourdesk-remote"
if [[ "$BUILD_ONLY" == 1 ]]; then exit 0; fi
echo "開啟 Client 桌面視窗……"
./bin/yourdesk-client -ui -signal "${YOURDESK_SIGNAL_URL:-wss://${YOURDESK_SIGNAL_HOST:-desktop.mars-cloud.com}:${YOURDESK_SIGNAL_PORT:-8080}/ws}" -room "${YOURDESK_ROOM:-}" "$@"
