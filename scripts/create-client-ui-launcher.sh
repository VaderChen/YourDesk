#!/bin/bash
set -euo pipefail
PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
/usr/bin/osacompile -o "$PROJECT_ROOT/clientUI.app" "$PROJECT_ROOT/scripts/client-ui-launcher.applescript"
if ! /usr/libexec/PlistBuddy -c 'Set :LSUIElement true' "$PROJECT_ROOT/clientUI.app/Contents/Info.plist" 2>/dev/null; then
 /usr/libexec/PlistBuddy -c 'Add :LSUIElement bool true' "$PROJECT_ROOT/clientUI.app/Contents/Info.plist"
fi
"$PROJECT_ROOT/scripts/sign-local.sh" "$PROJECT_ROOT/clientUI.app"
