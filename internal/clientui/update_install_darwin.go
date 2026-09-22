package clientui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"yourdesk/internal/prelogin"
)

func detachUpdateHelper(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
func prepareAutomaticUpdate(ctx context.Context, archive, _ string) error {
	serviceEnabled := prelogin.Status().Enabled
	if !strings.EqualFold(filepath.Ext(archive), ".dmg") {
		return fmt.Errorf("不支援的 macOS 更新套件")
	}
	target := "/Applications/YourDesk.app"
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	for p := filepath.Dir(executable); p != filepath.Dir(p); p = filepath.Dir(p) {
		if filepath.Base(p) == "YourDesk.app" && !strings.HasPrefix(p, "/Volumes/") && !strings.Contains(p, "/AppTranslocation/") {
			target = p
			break
		}
	}
	if info, err := os.Lstat(target); err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return fmt.Errorf("安裝位置不是一般 APP 目錄")
	}
	stage, err := os.MkdirTemp(filepath.Dir(target), ".YourDesk-update-")
	if err != nil {
		return fmt.Errorf("無法寫入安裝位置 %s，請手動安裝：%w", target, err)
	}
	handedOff := false
	defer func() {
		if !handedOff {
			os.RemoveAll(stage)
		}
	}()
	mount := filepath.Join(stage, "volume")
	if err = os.Mkdir(mount, 0700); err != nil {
		return err
	}
	if out, err := exec.CommandContext(ctx, "/usr/bin/hdiutil", "attach", "-readonly", "-nobrowse", "-mountpoint", mount, archive).CombinedOutput(); err != nil {
		return fmt.Errorf("無法掛載更新套件：%s", out)
	}
	defer exec.Command("/usr/bin/hdiutil", "detach", mount).Run()
	source := filepath.Join(mount, "YourDesk.app")
	bundleID, err := exec.CommandContext(ctx, "/usr/libexec/PlistBuddy", "-c", "Print :CFBundleIdentifier", filepath.Join(source, "Contents/Info.plist")).Output()
	if err != nil || strings.TrimSpace(string(bundleID)) != "com.yourdesk.desktop" {
		return fmt.Errorf("更新套件的 APP 識別不符")
	}
	staged := filepath.Join(stage, "YourDesk.app")
	if out, err := exec.CommandContext(ctx, "/usr/bin/ditto", source, staged).CombinedOutput(); err != nil {
		return fmt.Errorf("準備新版失敗：%s", out)
	}
	if out, err := exec.CommandContext(ctx, "/usr/bin/codesign", "--verify", "--deep", "--strict", staged).CombinedOutput(); err != nil {
		return fmt.Errorf("新版簽章驗證失敗：%s", out)
	}
	script := filepath.Join(stage, "install.sh")
	if err = os.WriteFile(script, []byte(macUpdateScript), 0700); err != nil {
		return err
	}
	service := "0"
	if serviceEnabled {
		service = "1"
	}
	args := []string{script, strconv.Itoa(os.Getpid()), target, stage, service, strconv.Itoa(os.Getuid())}
	if serviceEnabled {
		quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
		command := "/bin/sh"
		for _, arg := range append(append([]string{}, args...), "privileged") {
			command += " " + quote(arg)
		}
		if err = os.WriteFile(filepath.Join(stage, "elevate.scpt"), []byte("do shell script "+strconv.Quote(command)+" with administrator privileges"), 0600); err != nil {
			return err
		}
	}
	cmd := exec.Command("/bin/sh", args...)
	handedOff = true
	if err = startUpdateHelper(ctx, cmd, stage, serviceEnabled); err != nil {
		return err
	}
	return nil
}

const macUpdateScript = `#!/bin/sh
parent="$1"
target="$2"
stage="$3"
service="${4:-0}"
uid="${5:-0}"
backup="$stage/previous.app"
service_root="/Library/Application Support/YourDeskPrelogin"
service_app="$service_root/YourDesk.app"
service_backup="$service_root/.update-previous.app"
daemon="com.yourdesk.prelogin.broker"
agent="com.yourdesk.prelogin.agent"
service_stopped=0
app_changed=0
service_changed=0
start_service() {
 /bin/launchctl print "system/$daemon" >/dev/null 2>&1 || /bin/launchctl bootstrap system "/Library/LaunchDaemons/$daemon.plist" || return 1
 if [ "$uid" -ne 0 ]; then
  /bin/launchctl print "gui/$uid/$agent" >/dev/null 2>&1 || /bin/launchctl bootstrap "gui/$uid" "/Library/LaunchAgents/$agent.plist" || return 1
 fi
}
wait_service() {
 attempts=0
 healthy=0
 while [ "$attempts" -lt 15 ]; do
  if /bin/launchctl asuser "$uid" /usr/bin/sudo -u "#$uid" "$service_app/Contents/MacOS/yourdesk-client" --prelogin health >/dev/null 2>&1; then
   healthy=$((healthy+1)); [ "$healthy" -lt 3 ] || return 0
  else healthy=0; fi
  attempts=$((attempts+1)); sleep 1
 done
 return 1
}
open_app() {
 if [ "$service" = 1 ]; then
  /bin/launchctl asuser "$uid" /usr/bin/sudo -u "#$uid" /usr/bin/open "$target"
 else
  /usr/bin/open "$target"
 fi
}
stop_service() {
 if /bin/launchctl print "system/$daemon" >/dev/null 2>&1; then /bin/launchctl bootout "system/$daemon" || return 1; fi
 if /bin/launchctl print "gui/$uid/$agent" >/dev/null 2>&1; then /bin/launchctl bootout "gui/$uid/$agent" || return 1; fi
 /bin/launchctl unload -S LoginWindow "/Library/LaunchAgents/$agent.plist" >/dev/null 2>&1 || true
 sleep 6
}
fail() {
 echo "Automatic update failed: $1"
 if [ "$service_stopped" = 1 ]; then stop_service || true; fi
 if [ "$app_changed" = 1 ]; then
  /bin/mv "$target" "$stage/failed.app" 2>/dev/null || true
  [ ! -d "$backup" ] || /bin/mv "$backup" "$target"
 fi
 if [ "$service_changed" = 1 ] && [ -d "$service_backup" ]; then
  /bin/rm -rf "$service_app"
  /bin/mv "$service_backup" "$service_app"
 fi
 if [ "$service_stopped" = 1 ]; then start_service || echo "Service recovery failed; backup retained."; fi
 if [ "$app_changed" = 1 ] && [ -d "$target" ]; then open_app || echo "Cannot reopen previous app."; fi
 if [ "$service" != 1 ] && [ -f "$stage/ready" ]; then
  /usr/bin/osascript -e 'display alert "YourDesk 更新失敗" message "已嘗試恢復並重新開啟原本 APP。更新記錄保留於更新暫存目錄。"' || true
 fi
 exit 1
}
if [ "$service" = 1 ] && [ "${6:-}" != privileged ]; then
 /usr/bin/osascript "$stage/elevate.scpt" || {
  if [ -f "$stage/ready" ]; then /usr/bin/osascript -e 'display alert "YourDesk 更新失敗" message "已嘗試恢復原本服務。請檢查更新記錄並重新開啟 APP。"'; fi
  exit 1
 }
 exit 0
fi
if [ "$service" = 1 ]; then
 [ "$(/usr/bin/id -u)" = 0 ] || fail "Administrator authorization required"
 [ -d "$service_app" ] && [ ! -L "$service_root" ] && [ ! -L "$service_app" ] || fail "Invalid service installation"
 [ ! -e "$service_backup" ] || fail "Previous service recovery is pending"
fi
: > "$stage/ready"
i=0
while kill -0 "$parent" 2>/dev/null; do
 [ ! -f "$stage/cancelled" ] || fail "Update cancelled"
 i=$((i+1)); [ "$i" -lt 120 ] || fail "APP did not exit"
 sleep 1
done
[ ! -f "$stage/cancelled" ] || fail "Update cancelled"
if [ "$service" = 1 ]; then
 service_stopped=1
 stop_service || fail "Cannot stop service"
 /bin/mv "$service_app" "$service_backup" || fail "Cannot preserve service"
 service_changed=1
fi
if [ -e "$target" ]; then /bin/mv "$target" "$backup" || fail "Cannot preserve previous app"; fi
app_changed=1
/bin/mv "$stage/YourDesk.app" "$target" || fail "Cannot install new app"
if [ "$service" = 1 ]; then
 /usr/bin/ditto "$target" "$service_app" || fail "Cannot update service copy"
 /usr/sbin/chown -R root:wheel "$service_app" || fail "Cannot protect service owner"
 /bin/chmod -R go-w "$service_app" || fail "Cannot protect service permissions"
 /usr/bin/codesign --verify --deep --strict "$service_app" || fail "Service signature is invalid"
 start_service || fail "Cannot restart service"
 wait_service || fail "Service health check failed"
 open_app || fail "Cannot launch new app"
 /bin/rm -rf "$service_backup"
else
 open_app || fail "Cannot launch new app"
fi
/bin/rm -rf "$stage"
`
