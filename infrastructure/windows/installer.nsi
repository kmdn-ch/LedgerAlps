; =============================================================================
; LedgerAlps — NSIS Windows Installer
; Build with: makensis /DVERSION=v1.2.3 infrastructure\windows\installer.nsi
; Requires: NSIS 3.x, placed in same directory as ledgeralps-server.exe + ledgeralps-cli.exe
; =============================================================================

Unicode True

; --------------------------------------------------------------------------- ;
; Variables — override via /D flags                                           ;
; --------------------------------------------------------------------------- ;
!ifndef VERSION
  !define VERSION "dev"
!endif

!define PRODUCT_NAME     "LedgerAlps"
!define PRODUCT_VERSION  "${VERSION}"
!define PRODUCT_PUBLISHER "LedgerAlps"
!define PRODUCT_URL      "https://github.com/kmdn-ch/ledgeralps"
!define PRODUCT_EXE      "ledgeralps-server.exe"
!define PRODUCT_CLI      "ledgeralps-cli.exe"
!define SERVICE_NAME     "LedgerAlps"
!define SERVICE_DISPLAY  "LedgerAlps Accounting Server"
!define INSTALL_DIR      "$PROGRAMFILES64\LedgerAlps"
!define UNINSTALL_KEY    "Software\Microsoft\Windows\CurrentVersion\Uninstall\${PRODUCT_NAME}"
!define OUT_FILE         "LedgerAlps_Setup_${VERSION}_windows_amd64.exe"

; --------------------------------------------------------------------------- ;
; MUI2 configuration                                                          ;
; --------------------------------------------------------------------------- ;
!include "MUI2.nsh"
!include "WinMessages.nsh"

!define MUI_ABORTWARNING
!define MUI_FINISHPAGE_RUN          "$INSTDIR\${PRODUCT_EXE}"
!define MUI_FINISHPAGE_RUN_TEXT     "Start LedgerAlps server now"
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
Name            "${PRODUCT_NAME} ${PRODUCT_VERSION}"
OutFile         "${OUT_FILE}"
InstallDir      "${INSTALL_DIR}"
InstallDirRegKey HKLM "${UNINSTALL_KEY}" "InstallLocation"
RequestExecutionLevel admin
ShowInstDetails show
ShowUnInstDetails show

; --------------------------------------------------------------------------- ;
; Installer sections                                                          ;
; --------------------------------------------------------------------------- ;
Section "LedgerAlps (required)" SecMain
  SectionIn RO

  SetOutPath "$INSTDIR"
  File "ledgeralps-server.exe"
  File "ledgeralps-cli.exe"
  File "..\..\README.md"
  File "..\..\LICENSE"

  ; Create data directory for SQLite database and config
  CreateDirectory "$COMMONAPPDATA\LedgerAlps"

  ; Write example environment file if it doesn't exist
  IfFileExists "$COMMONAPPDATA\LedgerAlps\ledgeralps.env" env_exists env_missing
  env_missing:
    FileOpen $0 "$COMMONAPPDATA\LedgerAlps\ledgeralps.env.example" w
    FileWrite $0 "# LedgerAlps environment configuration$\r$\n"
    FileWrite $0 "# Copy this file to ledgeralps.env and fill in the values.$\r$\n"
    FileWrite $0 "$\r$\n"
    FileWrite $0 "# REQUIRED: Generate with: openssl rand -hex 32$\r$\n"
    FileWrite $0 "JWT_SECRET=CHANGE_ME_TO_A_32_CHAR_MINIMUM_SECRET$\r$\n"
    FileWrite $0 "$\r$\n"
    FileWrite $0 "# Server port (default: 8000)$\r$\n"
    FileWrite $0 "PORT=8000$\r$\n"
    FileWrite $0 "$\r$\n"
    FileWrite $0 "# SQLite database path$\r$\n"
    FileWrite $0 "SQLITE_PATH=$COMMONAPPDATA\LedgerAlps\ledgeralps.db$\r$\n"
    FileWrite $0 "$\r$\n"
    FileWrite $0 "# OR use PostgreSQL (comment out SQLITE_PATH above)$\r$\n"
    FileWrite $0 "# POSTGRES_DSN=postgres://user:password@localhost:5432/ledgeralps?sslmode=disable$\r$\n"
    FileWrite $0 "$\r$\n"
    FileWrite $0 "# CORS — allowed frontend origins (comma-separated)$\r$\n"
    FileWrite $0 "ALLOWED_ORIGINS=http://localhost:5173,http://localhost:3000$\r$\n"
    FileClose $0
  env_exists:

  ; Add to system PATH
  ReadRegStr $0 HKLM "SYSTEM\CurrentControlSet\Control\Session Manager\Environment" "Path"
  StrCpy $1 "$0;$INSTDIR"
  WriteRegExpandStr HKLM "SYSTEM\CurrentControlSet\Control\Session Manager\Environment" "Path" "$1"
  SendMessage ${HWND_BROADCAST} ${WM_WININICHANGE} 0 "STR:Environment" /TIMEOUT=5000

  ; Register Windows Service using sc.exe
  ; The service reads config from the env file
  ExecWait 'sc create "${SERVICE_NAME}" binPath= "\"$INSTDIR\${PRODUCT_EXE}\"" DisplayName= "${SERVICE_DISPLAY}" start= auto obj= LocalSystem'
  ExecWait 'sc description "${SERVICE_NAME}" "LedgerAlps Swiss SME Accounting Platform — double-entry bookkeeping with Swiss compliance (QR-bill, ISO 20022, TVA)."'
  ExecWait 'sc failure "${SERVICE_NAME}" reset= 60 actions= restart/5000/restart/10000/restart/30000'

  ; Start Menu shortcuts
  CreateDirectory "$SMPROGRAMS\${PRODUCT_NAME}"
  CreateShortcut "$SMPROGRAMS\${PRODUCT_NAME}\LedgerAlps Server.lnk" "$INSTDIR\${PRODUCT_EXE}" "" "$INSTDIR\${PRODUCT_EXE}"
  CreateShortcut "$SMPROGRAMS\${PRODUCT_NAME}\Open LedgerAlps.lnk" "http://localhost:8000" "" ""
  CreateShortcut "$SMPROGRAMS\${PRODUCT_NAME}\Uninstall LedgerAlps.lnk" "$INSTDIR\Uninstall.exe"

  ; Write uninstall registry keys
  WriteRegStr   HKLM "${UNINSTALL_KEY}" "DisplayName"      "${PRODUCT_NAME}"
  WriteRegStr   HKLM "${UNINSTALL_KEY}" "DisplayVersion"   "${PRODUCT_VERSION}"
  WriteRegStr   HKLM "${UNINSTALL_KEY}" "Publisher"        "${PRODUCT_PUBLISHER}"
  WriteRegStr   HKLM "${UNINSTALL_KEY}" "URLInfoAbout"     "${PRODUCT_URL}"
  WriteRegStr   HKLM "${UNINSTALL_KEY}" "InstallLocation"  "$INSTDIR"
  WriteRegStr   HKLM "${UNINSTALL_KEY}" "UninstallString"  '"$INSTDIR\Uninstall.exe"'
  WriteRegDWORD HKLM "${UNINSTALL_KEY}" "NoModify"         1
  WriteRegDWORD HKLM "${UNINSTALL_KEY}" "NoRepair"         1

  WriteUninstaller "$INSTDIR\Uninstall.exe"

  DetailPrint ""
  DetailPrint "Installation complete!"
  DetailPrint ""
  DetailPrint "NEXT STEPS:"
  DetailPrint "1. Edit $COMMONAPPDATA\LedgerAlps\ledgeralps.env.example"
  DetailPrint "   Set JWT_SECRET to a strong random value (openssl rand -hex 32)"
  DetailPrint "   Copy to $COMMONAPPDATA\LedgerAlps\ledgeralps.env"
  DetailPrint "2. Start the service: sc start ${SERVICE_NAME}"
  DetailPrint "3. Open http://localhost:8000"
SectionEnd

; --------------------------------------------------------------------------- ;
; Uninstaller                                                                 ;
; --------------------------------------------------------------------------- ;
Section "Uninstall"
  ; Stop and remove Windows Service
  ExecWait 'sc stop "${SERVICE_NAME}"'
  Sleep 2000
  ExecWait 'sc delete "${SERVICE_NAME}"'

  ; Remove files
  Delete "$INSTDIR\${PRODUCT_EXE}"
  Delete "$INSTDIR\${PRODUCT_CLI}"
  Delete "$INSTDIR\README.md"
  Delete "$INSTDIR\LICENSE"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir  "$INSTDIR"

  ; Remove Start Menu shortcuts
  Delete "$SMPROGRAMS\${PRODUCT_NAME}\*.lnk"
  RMDir  "$SMPROGRAMS\${PRODUCT_NAME}"

  ; Remove uninstall registry key
  DeleteRegKey HKLM "${UNINSTALL_KEY}"

  ; Note: data in $COMMONAPPDATA\LedgerAlps is preserved — user must manually remove
  DetailPrint ""
  DetailPrint "LedgerAlps has been uninstalled."
  DetailPrint "Your data in $COMMONAPPDATA\LedgerAlps has been preserved."
  DetailPrint "Delete that folder manually if you no longer need your data."
SectionEnd

