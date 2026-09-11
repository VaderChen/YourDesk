#!/bin/bash
# 本機啟動沿用 Client UI；不再啟動獨立訊號伺服器。
set -euo pipefail
PROJECT_ROOT="$(cd "$(dirname "$0")" && pwd)"
exec "$PROJECT_ROOT/runUITest.command" "$@"
