#!/bin/bash
set -euo pipefail
PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
STAGING="$(mktemp -d "$PROJECT_ROOT/.client-ui-sign.XXXXXX")"
trap 'rm -rf "$STAGING"' EXIT
APP="$STAGING/clientUI.app"
/usr/bin/osacompile -o "$APP" "$PROJECT_ROOT/scripts/client-ui-launcher.applescript"
# 沿用既有簽章識別碼 clientUI，不改成另一個 APP 身分。
/usr/libexec/PlistBuddy -c 'Add :CFBundleIdentifier string clientUI' "$APP/Contents/Info.plist"
/usr/libexec/PlistBuddy -c 'Add :LSUIElement bool true' "$APP/Contents/Info.plist"
"$PROJECT_ROOT/scripts/sign-local.sh" "$APP"
/usr/bin/codesign --verify --deep --strict "$APP"
# 沿用正式建置的公證流程；成功後才替換現有啟動器。
export YOURDESK_NOTARY_PROFILE="${YOURDESK_NOTARY_PROFILE:-VaderApp}"
python3 - "$PROJECT_ROOT/scripts/release.py" "$APP" <<'PYTHON'
import runpy, sys
from pathlib import Path
release = runpy.run_path(sys.argv[1])
release['notarize_app'](Path(sys.argv[2]))
PYTHON
# 先完成簽章再替換；若替換失敗保留原啟動器。
if [[ -e "$PROJECT_ROOT/clientUI.app.bak" ]]; then
 echo 'clientUI.app.bak 已存在，請先確認備份。' >&2
 exit 1
fi
if [[ -e "$PROJECT_ROOT/clientUI.app" ]]; then
 mv "$PROJECT_ROOT/clientUI.app" "$PROJECT_ROOT/clientUI.app.bak"
fi
if mv "$APP" "$PROJECT_ROOT/clientUI.app"; then
 rm -rf "$PROJECT_ROOT/clientUI.app.bak"
else
 if [[ -e "$PROJECT_ROOT/clientUI.app.bak" ]]; then mv "$PROJECT_ROOT/clientUI.app.bak" "$PROJECT_ROOT/clientUI.app"; fi
 exit 1
fi
