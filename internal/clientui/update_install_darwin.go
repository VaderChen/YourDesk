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
func prepareAutomaticUpdate(ctx context.Context, archive string) error {
	if prelogin.Status().Enabled {
		return fmt.Errorf("請先停用未登入開機，再更新 APP；更新後重新啟用服務。")
	}
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
	cmd := exec.Command("/bin/sh", script, strconv.Itoa(os.Getpid()), target, stage)
	handedOff = true
	if err = startUpdateHelper(ctx, cmd, stage); err != nil {
		return err
	}
	return nil
}

const macUpdateScript = `#!/bin/sh
parent="$1"
target="$2"
stage="$3"
backup="$stage/previous.app"
fail() {
 echo "Automatic update failed: $1"
 /usr/bin/osascript -e 'display alert "YourDesk 更新未完成" message "請保留下載的安裝包並手動安裝。更新記錄位於安裝目錄中的 .YourDesk-update 資料夾。"' >/dev/null 2>&1
 exit 1
}
: > "$stage/ready"
i=0
while kill -0 "$parent" 2>/dev/null; do
 i=$((i+1)); [ "$i" -lt 120 ] || fail "APP did not exit"
 sleep 1
done
if [ -e "$target" ]; then /bin/mv "$target" "$backup" || fail "Cannot preserve previous app"; fi
if ! /bin/mv "$stage/YourDesk.app" "$target"; then
 [ ! -d "$backup" ] || /bin/mv "$backup" "$target"
 fail "Cannot install new app"
fi
if ! /usr/bin/open "$target"; then
 /bin/mv "$target" "$stage/failed.app"
 if [ -d "$backup" ]; then /bin/mv "$backup" "$target"; /usr/bin/open "$target"; fi
 fail "Cannot launch new app"
fi
/bin/rm -rf "$stage"
`
