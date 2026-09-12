; Windows 每位使用者安裝程式；不刪除設定或下載的資料。
Unicode true
RequestExecutionLevel user
!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "x64.nsh"
Name "YourDesk"
OutFile "${OUTPUT_FILE}"
InstallDir "$LOCALAPPDATA\Programs\YourDesk"
InstallDirRegKey HKCU "Software\YourDesk" "InstallDir"
SetCompressor /SOLID lzma
VIProductVersion "${NUMERIC_VERSION}"
VIAddVersionKey "ProductName" "YourDesk"
VIAddVersionKey "FileDescription" "YourDesk Installer"
VIAddVersionKey "FileVersion" "${APP_VERSION}"
VIAddVersionKey "LegalCopyright" "YourDesk"
!define MUI_ICON "../assets/branding/yourdesk.ico"
!define MUI_UNICON "../assets/branding/yourdesk.ico"
!define MUI_ABORTWARNING
!define MUI_FINISHPAGE_RUN "$INSTDIR\YourDesk.exe"
!define MUI_LANGDLL_REGISTRY_ROOT HKCU
!define MUI_LANGDLL_REGISTRY_KEY "Software\YourDesk"
!define MUI_LANGDLL_REGISTRY_VALUENAME "InstallerLanguage"
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "TradChinese"
!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "Japanese"
!insertmacro MUI_LANGUAGE "Korean"
LangString InUse ${LANG_TRADCHINESE} "請先從 Tray 關閉 YourDesk，並關閉所有遠端顯示，再按重試。"
LangString InUse ${LANG_ENGLISH} "Quit YourDesk from the tray and close all Remote Display windows, then retry."
LangString InUse ${LANG_JAPANESE} "トレイから YourDesk を終了し、すべてのリモート画面 を閉じてから再試行してください。"
LangString InUse ${LANG_KOREAN} "트레이에서 YourDesk를 종료하고 모든 원격 화면를 닫은 후 다시 시도하세요."
LangString WrongArch ${LANG_TRADCHINESE} "此安裝程式不適用於目前的 Windows 架構。"
LangString WrongArch ${LANG_ENGLISH} "This installer does not match your Windows architecture."
LangString WrongArch ${LANG_JAPANESE} "このインストーラーは Windows のアーキテクチャに対応していません。"
LangString WrongArch ${LANG_KOREAN} "이 설치 프로그램은 Windows 아키텍처와 맞지 않습니다."
LangString ServiceActive ${LANG_TRADCHINESE} "請先在 YourDesk 進階設定關閉「登入前啟動」，再更新或解除安裝。"
LangString ServiceActive ${LANG_ENGLISH} "Turn off Start before login in YourDesk Advanced Settings before updating or uninstalling."
LangString ServiceActive ${LANG_JAPANESE} "更新またはアンインストールの前に、YourDesk の詳細設定でログイン前の起動を無効にしてください。"
LangString ServiceActive ${LANG_KOREAN} "업데이트 또는 제거 전에 YourDesk 고급 설정에서 로그인 전 시작을 꺼 주세요."

!macro CheckPreloginService
 ReadRegStr $0 HKLM "SYSTEM\CurrentControlSet\Services\YourDeskPrelogin" "ImagePath"
 ${If} $0 != ""
  IfSilent +2 0
  MessageBox MB_OK|MB_ICONEXCLAMATION "$(ServiceActive)"
  SetErrorLevel 3
  Abort
 ${EndIf}
!macroend

!macro CheckFileClosed FILE
retry_${FILE}:
 IfFileExists "$INSTDIR\${FILE}" 0 done_${FILE}
 System::Call 'kernel32::CreateFileW(w "$INSTDIR\${FILE}", i 0x40000000, i 0, p 0, i 3, i 0, p 0) p.r0'
 ${If} $0 == -1
  IfSilent 0 +3
   SetErrorLevel 2
   Abort
  MessageBox MB_RETRYCANCEL|MB_ICONEXCLAMATION "$(InUse)" IDRETRY retry_${FILE}
  Abort
 ${EndIf}
 System::Call 'kernel32::CloseHandle(p r0)'
done_${FILE}:
!macroend

Function .onInit
 SetShellVarContext current
 ${IfNot} ${Silent}
  !insertmacro MUI_LANGDLL_DISPLAY
 ${EndIf}
!if "${APP_ARCH}" == "arm64"
 ${IfNot} ${IsNativeARM64}
!else
 ${IfNot} ${IsNativeAMD64}
!endif
  MessageBox MB_OK|MB_ICONSTOP "$(WrongArch)"
  Abort
 ${EndIf}
FunctionEnd
Function un.onInit
 SetShellVarContext current
 !insertmacro MUI_UNGETLANGUAGE
FunctionEnd
Section "YourDesk"
 !insertmacro CheckPreloginService
 !insertmacro CheckFileClosed "YourDesk.exe"
 !insertmacro CheckFileClosed "yourdesk-client.exe"
 !insertmacro CheckFileClosed "yourdesk-remote.exe"
 ClearErrors
 SetOutPath "$INSTDIR"
 File "${PAYLOAD_DIR}/YourDesk.exe"
 File "${PAYLOAD_DIR}/yourdesk-client.exe"
 File "${PAYLOAD_DIR}/yourdesk-remote.exe"
 File "${PAYLOAD_DIR}/avcodec-62.dll"
 File "${PAYLOAD_DIR}/avutil-60.dll"
 File "${PAYLOAD_DIR}/swscale-9.dll"
 File /r "${PAYLOAD_DIR}/ThirdPartyLicenses"
 File "${PAYLOAD_DIR}/README.txt"
 File "${PAYLOAD_DIR}/LICENSE.md"
 File "${PAYLOAD_DIR}/LICENSE.en.md"
 File "${PAYLOAD_DIR}/LICENSE.ja.md"
 File "${PAYLOAD_DIR}/LICENSE.ko.md"
 Delete "$INSTDIR\使用說明.txt"
 ${If} ${Errors}
  SetErrorLevel 3
  Abort
 ${EndIf}
 WriteUninstaller "$INSTDIR\Uninstall.exe"
 CreateDirectory "$SMPROGRAMS\YourDesk"
 CreateShortcut "$SMPROGRAMS\YourDesk\YourDesk.lnk" "$INSTDIR\YourDesk.exe"
 CreateShortcut "$DESKTOP\YourDesk.lnk" "$INSTDIR\YourDesk.exe"
 WriteRegStr HKCU "Software\YourDesk" "InstallDir" "$INSTDIR"
 WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\YourDesk" "DisplayName" "YourDesk"
 WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\YourDesk" "DisplayVersion" "${APP_VERSION}"
 WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\YourDesk" "InstallLocation" "$INSTDIR"
 WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\YourDesk" "DisplayIcon" "$INSTDIR\YourDesk.exe"
 WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\YourDesk" "UninstallString" '$\"$INSTDIR\Uninstall.exe$\"'
 WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\YourDesk" "NoModify" 1
 WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\YourDesk" "NoRepair" 1
SectionEnd
Section "Uninstall"
 !insertmacro CheckPreloginService
 DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "YourDesk"
 DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run" "YourDesk"
 !insertmacro CheckFileClosed "YourDesk.exe"
 !insertmacro CheckFileClosed "yourdesk-client.exe"
 !insertmacro CheckFileClosed "yourdesk-remote.exe"
 Delete "$INSTDIR\YourDesk.exe"
 Delete "$INSTDIR\yourdesk-client.exe"
 Delete "$INSTDIR\yourdesk-remote.exe"
 Delete "$INSTDIR\avcodec-62.dll"
 Delete "$INSTDIR\avutil-60.dll"
 Delete "$INSTDIR\swscale-9.dll"
 RMDir /r "$INSTDIR\ThirdPartyLicenses"
 Delete "$INSTDIR\README.txt"
 Delete "$INSTDIR\LICENSE.md"
 Delete "$INSTDIR\LICENSE.en.md"
 Delete "$INSTDIR\LICENSE.ja.md"
 Delete "$INSTDIR\LICENSE.ko.md"
 Delete "$INSTDIR\使用說明.txt"
 Delete "$INSTDIR\Uninstall.exe"
 Delete "$DESKTOP\YourDesk.lnk"
 Delete "$SMPROGRAMS\YourDesk\YourDesk.lnk"
 RMDir "$SMPROGRAMS\YourDesk"
 RMDir "$INSTDIR"
 DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\YourDesk"
 DeleteRegValue HKCU "Software\YourDesk" "InstallDir"
SectionEnd
