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
 !insertmacro CheckFileClosed "YourDesk.exe"
 !insertmacro CheckFileClosed "yourdesk-client.exe"
 !insertmacro CheckFileClosed "yourdesk-remote.exe"
 ClearErrors
 SetOutPath "$INSTDIR"
 File "${PAYLOAD_DIR}/YourDesk.exe"
 File "${PAYLOAD_DIR}/yourdesk-client.exe"
 File "${PAYLOAD_DIR}/yourdesk-remote.exe"
 File /r "${PAYLOAD_DIR}/ThirdPartyLicenses"
 File "${PAYLOAD_DIR}/README.txt"
 File "${PAYLOAD_DIR}/使用說明.txt"
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
 !insertmacro CheckFileClosed "YourDesk.exe"
 !insertmacro CheckFileClosed "yourdesk-client.exe"
 !insertmacro CheckFileClosed "yourdesk-remote.exe"
 Delete "$INSTDIR\YourDesk.exe"
 Delete "$INSTDIR\yourdesk-client.exe"
 Delete "$INSTDIR\yourdesk-remote.exe"
 RMDir /r "$INSTDIR\ThirdPartyLicenses"
 Delete "$INSTDIR\README.txt"
 Delete "$INSTDIR\使用說明.txt"
 Delete "$INSTDIR\Uninstall.exe"
 Delete "$DESKTOP\YourDesk.lnk"
 Delete "$SMPROGRAMS\YourDesk\YourDesk.lnk"
 RMDir "$SMPROGRAMS\YourDesk"
 RMDir "$INSTDIR"
 DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\YourDesk"
 DeleteRegValue HKCU "Software\YourDesk" "InstallDir"
SectionEnd
