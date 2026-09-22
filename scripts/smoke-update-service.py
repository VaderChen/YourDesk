#!/usr/bin/env python3
"""隔離執行產品更新腳本；驗證服務健康、回復與重開，不碰真實 APP／服務。"""
from pathlib import Path
import subprocess
import tempfile
import shlex

root = Path(__file__).resolve().parents[1]
source = (root / "internal/clientui/update_install_darwin.go").read_text()
script = source.split("const macUpdateScript = `", 1)[1].rsplit("`", 1)[0]
subprocess.run(["/bin/sh", "-n"], input=script, text=True, check=True)
for enabled, failure in [(False, ""), (True, ""), (True, "copy"), (True, "health"), (False, "launch"), (True, "launch")]:
    with tempfile.TemporaryDirectory(prefix="yd-update-smoke-") as temp:
        base = Path(temp)
        stage, target, service = base/"stage", base/"YourDesk.app", base/"service"
        launches = base/"launches"
        for path, value in [(stage/"YourDesk.app", "new"), (target, "old"), (service/"YourDesk.app", "old")]:
            path.mkdir(parents=True)
            (path/"version").write_text(value)
            helper = path/"Contents/MacOS/yourdesk-client"
            helper.parent.mkdir(parents=True)
            helper.write_text("#!/bin/sh\nexit " + ("1" if failure == "health" and value == "new" else "0") + "\n")
            helper.chmod(0o700)
        (service/"config.json").write_text("preserve")
        launch = base/"open"
        launch.write_text("#!/bin/sh\nv=$(cat \"$1/version\")\necho \"$v\" >> " + shlex.quote(str(launches)) + ("\n[ \"$v\" = old ]\n" if failure == "launch" else "\nexit 0\n"))
        launch.chmod(0o700)
        launchctl = base/"launchctl"
        launchctl.write_text('#!/bin/sh\nif [ "$1" = asuser ]; then shift 2; exec "$@"; fi\nexit 0\n')
        launchctl.chmod(0o700)
        sudo = base/"sudo"
        sudo.write_text('#!/bin/sh\nshift 2\nexec "$@"\n'); sudo.chmod(0o700)
        patched = script.replace('/Library/Application Support/YourDeskPrelogin', str(service))
        patched = patched.replace('sleep 6', ':').replace('sleep 1', ':').replace('$(/usr/bin/id -u)', '0')
        patched = patched.replace('/bin/launchctl', shlex.quote(str(launchctl))).replace('/usr/bin/sudo', shlex.quote(str(sudo)))
        patched = patched.replace('/usr/sbin/chown', '/usr/bin/true').replace('/usr/bin/codesign', '/usr/bin/true')
        patched = patched.replace('/usr/bin/open', shlex.quote(str(launch))).replace('/usr/bin/osascript', '/usr/bin/true')
        patched = patched.replace('/usr/bin/ditto', '/usr/bin/false' if failure == "copy" else '/bin/cp -R')
        result = subprocess.run(['/bin/sh', '-s', '99999999', str(target), str(stage), str(int(enabled)), '501', 'privileged'], input=patched, text=True, capture_output=True, timeout=10)
        assert (result.returncode != 0) == bool(failure), result.stdout + result.stderr
        assert (target/"version").read_text() == ("old" if failure else "new")
        assert (service/"YourDesk.app"/"version").read_text() == ("new" if enabled and not failure else "old")
        assert (service/"config.json").read_text() == "preserve"
        if failure == "launch": assert launches.read_text().splitlines() == ["new", "old"]
        if failure == "health": assert launches.read_text().splitlines() == ["old"]
        if not failure: assert not stage.exists()
print("PASS: 一般／服務更新、拷貝失敗、服務啟動後無回應、啟動新版失敗、恢復並重開舊版")
