#!/bin/bash
set -Eeuo pipefail

# 使用遠端配對服務，同時啟動本機 Client 與 Remote。
ROOT="$(cd "$(dirname "$0")" && pwd)"
exec "$ROOT/remoteRun.command" --with-client "$@"
