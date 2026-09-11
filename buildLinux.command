#!/bin/zsh
set -euo pipefail
# 交叉編譯 Linux x64 與 arm64 命令列版本，不需要桌面 UI 工具鏈。
PROJECT_ROOT="${0:A:h}"
cd "$PROJECT_ROOT"
export YOURDESK_BUILD_TARGETS="linux/amd64,linux/arm64"
exec python3 "$PROJECT_ROOT/scripts/release.py" build "$@"
