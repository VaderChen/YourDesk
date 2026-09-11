#!/usr/bin/env bash
# 本機與發行版共用固定 Developer ID，拒絕默默退回 ad-hoc。
set -euo pipefail
[[ "$(uname -s)" == Darwin ]] || exit 0
SIGNING_IDENTITY="${YOURDESK_CODESIGN_IDENTITY:-}"
if [[ -z "$SIGNING_IDENTITY" ]]; then
  SIGNING_IDENTITY="$(security find-identity -v -p codesigning | sed -nE 's/^[[:space:]]*[0-9]+\) [0-9A-F]+ "(Developer ID Application:[^"]+)"$/\1/p' | head -n 1)"
fi
if [[ -z "$SIGNING_IDENTITY" || "$SIGNING_IDENTITY" == '-' ]]; then
  echo '找不到固定 Developer ID 簽章；請設定 YOURDESK_CODESIGN_IDENTITY。已停止，避免重新產生 ad-hoc 授權身分。' >&2
  exit 1
fi
for executable in "$@"; do
  codesign --force --sign "$SIGNING_IDENTITY" --options runtime --timestamp "$executable"
  codesign --verify --strict "$executable"
done
