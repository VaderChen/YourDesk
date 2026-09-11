#!/bin/bash
# 給 clientUI.app 使用；所有建置及執行輸出留在本機記錄檔。
set -Eeuo pipefail
PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
mkdir -p "$PROJECT_ROOT/.local-run"
LOG_PATH="$PROJECT_ROOT/.local-run/client-ui.log"
LOCK_PATH="$PROJECT_ROOT/.local-run/client-ui-launch.lock"
# 以 flock 替代工具在 macOS 不一定存在，使用原子目錄與 PID 偵測。
if ! mkdir "$LOCK_PATH" 2>/dev/null; then
  if [[ -f "$LOCK_PATH/pid" ]]; then
    read -r owner < "$LOCK_PATH/pid"
    if [[ "$owner" =~ ^[0-9]+$ ]] && kill -0 "$owner" 2>/dev/null; then exit 0; fi
    rm -f "$LOCK_PATH/pid"
    rmdir "$LOCK_PATH" 2>/dev/null || exit 0
    mkdir "$LOCK_PATH" 2>/dev/null || exit 0
  else
    exit 0
  fi
fi
printf '%s\n' "$$" > "$LOCK_PATH/pid"
trap 'rm -f "$LOCK_PATH/pid"; rmdir "$LOCK_PATH" 2>/dev/null || true' EXIT
if YOURDESK_TEST_UPDATE=1 /bin/bash "$PROJECT_ROOT/runUITest.command" >"$LOG_PATH" 2>&1 </dev/null; then
  exit 0
else
  result=$?
  /usr/bin/osascript - "$LOG_PATH" <<'APPLESCRIPT'
on run argv
 display alert "YourDesk 啟動或執行失敗" message ("請查看記錄檔：" & item 1 of argv) as critical
end run
APPLESCRIPT
  exit "$result"
fi
