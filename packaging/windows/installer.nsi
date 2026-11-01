; WaterStick Windows NSIS Installer Script
; Pure standard NSIS - no third-party plugins required

!define APP_NAME "WaterStick"
!define APP_PUBLISHER "MedCy1"
!define APP_URL "https://github.com/MedCy1/WaterStick"
!define APP_EXE "waterstick-tui.exe"
!define APP_CLI_EXE "waterstick.exe"

!ifndef APP_VERSION
  !define APP_VERSION "1.0.0"
!endif

Name "${APP_NAME} ${APP_VERSION}"
OutFile "waterstick-setup-${APP_VERSION}-windows-amd64.exe"
InstallDir "$LOCALAPPDATA\Programs\WaterStick"
InstallDirRegKey HKCU "Software\${APP_NAME}" "InstallDir"
RequestExecutionLevel user

; Modern UI 2
!include "MUI2.nsh"

!define MUI_ABORTWARNING

; Installer Pages
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!define MUI_FINISHPAGE_RUN "$INSTDIR\${APP_EXE}"
!define MUI_FINISHPAGE_RUN_TEXT "Launch WaterStick Dashboard"
!insertmacro MUI_PAGE_FINISH

; Uninstaller Pages
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"

Section "WaterStick Core" SecCore
  SetOutPath "$INSTDIR"

  ; Install binaries and documentation
  File "waterstick.exe"
  File "waterstick-tui.exe"
  File "README.md"
  File "LICENSE"

  ; Create uninstaller
  WriteUninstaller "$INSTDIR\uninstall.exe"

  ; Registry configuration for Add/Remove Programs
  WriteRegStr HKCU "Software\${APP_NAME}" "InstallDir" "$INSTDIR"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}" "DisplayName" "${APP_NAME}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}" "DisplayVersion" "${APP_VERSION}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}" "Publisher" "${APP_PUBLISHER}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}" "HelpLink" "${APP_URL}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}" "NoModify" 1
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}" "NoRepair" 1

  ; Start Menu Shortcuts
  CreateDirectory "$SMPROGRAMS\${APP_NAME}"
  CreateShortcut "$SMPROGRAMS\${APP_NAME}\${APP_NAME} Dashboard.lnk" "$INSTDIR\${APP_EXE}"
  CreateShortcut "$SMPROGRAMS\${APP_NAME}\${APP_NAME} CLI.lnk" "$INSTDIR\${APP_CLI_EXE}"
  CreateShortcut "$SMPROGRAMS\${APP_NAME}\Uninstall ${APP_NAME}.lnk" "$INSTDIR\uninstall.exe"

  ; Desktop Shortcut
  CreateShortcut "$DESKTOP\${APP_NAME}.lnk" "$INSTDIR\${APP_EXE}"
SectionEnd

Section "Uninstall"
  ; Remove Desktop & Start Menu Shortcuts
  Delete "$DESKTOP\${APP_NAME}.lnk"
  Delete "$SMPROGRAMS\${APP_NAME}\*.*"
  RMDir "$SMPROGRAMS\${APP_NAME}"

  ; Remove files
  Delete "$INSTDIR\waterstick.exe"
  Delete "$INSTDIR\waterstick-tui.exe"
  Delete "$INSTDIR\README.md"
  Delete "$INSTDIR\LICENSE"
  Delete "$INSTDIR\uninstall.exe"

  ; Clean registry
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}"
  DeleteRegKey HKCU "Software\${APP_NAME}"

  ; Remove installation directory
  RMDir "$INSTDIR"
SectionEnd
