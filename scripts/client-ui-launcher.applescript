on run
 set appPath to POSIX path of (path to me)
 set projectRoot to do shell script "/usr/bin/dirname " & quoted form of appPath
 do shell script "/usr/bin/nohup /bin/bash " & quoted form of (projectRoot & "/scripts/client-ui-background.sh") & " </dev/null >/dev/null 2>&1 &"
end run
