; =============================================================================
; LedgerAlps — NSIS Windows Installer
; Build with:
;   makensis /DVERSION=1.2.3 infrastructure\windows\installer.nsi
;
; Expected files in infrastructure\windows\ before running makensis:
;   ledgeralps.exe         (launcher / GUI entry point, built with -H=windowsgui)
;   ledgeralps-server.exe  (API + static-file server)
;   ledgeralps-cli.exe     (admin CLI)
;   dist\                  (React frontend build — frontend/dist/ from repo)
;     index.html
;     assets\
;       ...
; =============================================================================

Unicode True

; --------------------------------------------------------------------------- ;
; Variables — override via /D flags                                           ;
; --------------------------------------------------------------------------- ;
!ifndef VERSION
  !define VERSION "dev"
!endif

!define PRODUCT_NAME      "LedgerAlps"
!define PRODUCT_VERSION   "${VERSION}"
!define PRODUCT_PUBLISHER "LedgerAlps"
!define PRODUCT_URL       "https://github.com/kmdn-ch/ledgeralps"
!define LAUNCHER_EXE      "ledgeralps.exe"
!define SERVER_EXE        "ledgeralps-server.exe"
!define CLI_EXE           "ledgeralps-cli.exe"
!define INSTALL_DIR       "$PROGRAMFILES64\LedgerAlps"
!define UNINSTALL_KEY     "Software\Microsoft\Windows\CurrentVersion\Uninstall\${PRODUCT_NAME}"
!define OUT_FILE          "LedgerAlps_Setup_${VERSION}_windows_amd64.exe"

; --------------------------------------------------------------------------- ;
; MUI2 configuration                                                          ;
; --------------------------------------------------------------------------- ;
!include "MUI2.nsh"
!include "WinMessages.nsh"

!define MUI_ABORTWARNING

; On the Finish page, offer to launch the app (via the launcher).
!define MUI_FINISHPAGE_RUN          "$INSTDIR\${LAUNCHER_EXE}"
!define MUI_FINISHPAGE_RUN_TEXT     "Launch LedgerAlps"
!define MUI_FINISHPAGE_SHOWREADME   "$INSTDIR\README.md"
!define MUI_FINISHPAGE_SHOWREADME_TEXT "View README"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "..\..\LICENSE"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "French"

; --------------------------------------------------------------------------- ;
; Installer metadata                                                          ;
; --------------------------------------------------------------------------- ;
Name             "${PRODUCT_NAME} ${PRODUCT_VERSION}"
OutFile          "${OUT_FILE}"
InstallDir       "${INSTALL_DIR}"
InstallDirRegKey HKLM "${UNINSTALL_KEY}" "InstallLocation"
RequestExecutionLevel admin
ShowInstDetails show
ShowUnInstDetails show

; --------------------------------------------------------------------------- ;
; Pre-install: stop any running instance                                      ;
; --------------------------------------------------------------------------- ;
Function .onInit
  ; Kill any running server or launcher so files can be replaced.
  nsExec::ExecToLog 'taskkill /f /im "${SERVER_EXE}"'
  nsExec::ExecToLog 'taskkill /f /im "${LAUNCHER_EXE}"'
  Sleep 1000
FunctionEnd

; --------------------------------------------------------------------------- ;
; Installer section                                                           ;
; --------------------------------------------------------------------------- ;
Section "LedgerAlps (required)" SecMain
  SectionIn RO

  SetOutPath "$INSTDIR"
  File "${LAUNCHER_EXE}"
  File "${SERVER_EXE}"
  File "${CLI_EXE}"
  File "..\..\README.md"
  File "..\..\LICENSE"

  ; Note: the React frontend is embedded inside ledgeralps-server.exe (Go embed).
  ; No separate dist\ folder is needed in the install directory.

  ; ── Shortcuts ──────────────────────────────────────────────────────────── ;
  ; Start Menu
  CreateDirectory "$SMPROGRAMS\${PRODUCT_NAME}"
  CreateShortcut "$SMPROGRAMS\${PRODUCT_NAME}\LedgerAlps.lnk" \
    "$INSTDIR\${LAUNCHER_EXE}" "" "$INSTDIR\${LAUNCHER_EXE}" 0 \
    SW_SHOWNORMAL "" "Open LedgerAlps"
  CreateShortcut "$SMPROGRAMS\${PRODUCT_NAME}\Uninstall LedgerAlps.lnk" \
    "$INSTDIR\Uninstall.exe"

  ; Desktop shortcut
  CreateShortcut "$DESKTOP\LedgerAlps.lnk" \
    "$INSTDIR\${LAUNCHER_EXE}" "" "$INSTDIR\${LAUNCHER_EXE}" 0 \
    SW_SHOWNORMAL "" "Open LedgerAlps"

  ; ── Registry — uninstall entry ─────────────────────────────────────────── ;
  WriteRegStr   HKLM "${UNINSTALL_KEY}" "DisplayName"      "${PRODUCT_NAME}"
  WriteRegStr   HKLM "${UNINSTALL_KEY}" "DisplayVersion"   "${PRODUCT_VERSION}"
  WriteRegStr   HKLM "${UNINSTALL_KEY}" "Publisher"        "${PRODUCT_PUBLISHER}"
  WriteRegStr   HKLM "${UNINSTALL_KEY}" "URLInfoAbout"     "${PRODUCT_URL}"
  WriteRegStr   HKLM "${UNINSTALL_KEY}" "InstallLocation"  "$INSTDIR"
  WriteRegStr   HKLM "${UNINSTALL_KEY}" "UninstallString"  '"$INSTDIR\Uninstall.exe"'
  WriteRegDWORD HKLM "${UNINSTALL_KEY}" "NoModify"         1
  WriteRegDWORD HKLM "${UNINSTALL_KEY}" "NoRepair"         1
  WriteRegStr   HKLM "${UNINSTALL_KEY}" "DisplayIcon"      "$INSTDIR\${LAUNCHER_EXE}"

  WriteUninstaller "$INSTDIR\Uninstall.exe"

  DetailPrint ""
  DetailPrint "Installation complete."
  DetailPrint "Launch LedgerAlps from the Desktop or Start Menu."
  DetailPrint "On first launch a setup wizard will open in your browser."
SectionEnd

; --------------------------------------------------------------------------- ;
; Uninstaller                                                                 ;
; --------------------------------------------------------------------------- ;
Section "Uninstall"
  ; Stop any running server
  nsExec::ExecToLog 'taskkill /f /im "${SERVER_EXE}"'
  nsExec::ExecToLog 'taskkill /f /im "${LAUNCHER_EXE}"'
  Sleep 500

  ; Remove installed files
  Delete "$INSTDIR\${LAUNCHER_EXE}"
  Delete "$INSTDIR\${SERVER_EXE}"
  Delete "$INSTDIR\${CLI_EXE}"
  Delete "$INSTDIR\README.md"
  Delete "$INSTDIR\LICENSE"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir  "$INSTDIR"

  ; Remove shortcuts
  Delete "$SMPROGRAMS\${PRODUCT_NAME}\*.lnk"
  RMDir  "$SMPROGRAMS\${PRODUCT_NAME}"
  Delete "$DESKTOP\LedgerAlps.lnk"

  ; Remove uninstall registry key
  DeleteRegKey HKLM "${UNINSTALL_KEY}"

  ; NOTE: %APPDATA%\LedgerAlps (config.json + database) is intentionally
  ; preserved so user data survives an uninstall/reinstall cycle.
  DetailPrint ""
  DetailPrint "LedgerAlps has been uninstalled."
  DetailPrint "Your data in %APPDATA%\LedgerAlps has been preserved."
SectionEnd
