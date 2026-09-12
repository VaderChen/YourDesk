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
  # 不繼承 Go／AppleScript 工具產生的臨時識別碼；使用固定檔名或 Bundle ID。
  if [[ -d "$executable" && "$executable" == *.app ]]; then
    identifier="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$executable/Contents/Info.plist")"
  else
    identifier="$(basename "$executable")"
  fi
  [[ -n "$identifier" ]] || { echo '缺少固定簽章識別碼。' >&2; exit 1; }
  codesign --force --sign "$SIGNING_IDENTITY" --identifier "$identifier" --options runtime --timestamp "$executable"
  codesign --verify --deep --strict "$executable"
done
