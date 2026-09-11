#!/usr/bin/env bash
# WinPE 獨立入口與正式打包共用建置流程，不執行目標程式或修改 PE 映像。
set -euo pipefail
PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
exec python3 "$PROJECT_ROOT/scripts/release.py" winpe "$@"
