#!/usr/bin/env bash
# 舊入口沿用新的跨平台建置流程，避免 CGO 設定分歧。
set -euo pipefail
PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
exec python3 "$PROJECT_ROOT/scripts/release.py" build "$@"
