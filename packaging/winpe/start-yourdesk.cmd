@echo off
setlocal
cd /d "%~dp0"
if exist "%SystemRoot%\System32\wpeinit.exe" (
  echo Initializing WinPE networking...
  "%SystemRoot%\System32\wpeinit.exe"
  if errorlevel 1 (
    echo Network initialization failed. Check the network driver and IP settings.
    pause
    exit /b 1
  )
)
start "" /wait "%~dp0yourdesk-winpe.exe" %*
endlocal
