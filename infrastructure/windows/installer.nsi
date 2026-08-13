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

; The welcome and finish titles hold two lines by default. "Bienvenue dans le
; programme d'installation de LedgerAlps 1.4.4-rc3" does not fit, and the
; overflow is clipped mid-word rather than wrapped — the version number simply
; vanished. These give the title area a third line.
!define MUI_WELCOMEPAGE_TITLE_3LINES
!define MUI_FINISHPAGE_TITLE_3LINES

; On the Finish page, offer to launch the app (via the launcher).
!define MUI_FINISHPAGE_RUN          "$INSTDIR\${LAUNCHER_EXE}"
!define MUI_FINISHPAGE_RUN_TEXT     "$(RunApp)"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "..\..\LICENSE"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "French"
!insertmacro MUI_LANGUAGE "German"
!insertmacro MUI_LANGUAGE "Italian"

; --------------------------------------------------------------------------- ;
; Localised strings                                                           ;
;                                                                             ;
; This file is UTF-8 *with BOM* — that is load-bearing. NSIS 3 only reads a    ;
; script as UTF-8 when a BOM is present; without one it falls back to the      ;
; system ANSI codepage and every accented character is mangled (the classic    ;
; "donnÃ©es" for "données"). Keep the BOM when editing.                        ;
;                                                                             ;
; LangStrings must be declared after the MUI_LANGUAGE macros above.           ;
; --------------------------------------------------------------------------- ;
LangString DeleteDataQuestion ${LANG_ENGLISH} \
  "Do you want to delete your accounting data?$\n(database, configuration, logs)"
LangString DeleteDataQuestion ${LANG_FRENCH} \
  "Souhaitez-vous supprimer vos données comptables ?$\n(base de données, configuration, journaux)"
LangString DeleteDataQuestion ${LANG_GERMAN} \
  "Möchten Sie Ihre Buchhaltungsdaten löschen?$\n(Datenbank, Konfiguration, Protokolle)"
LangString DeleteDataQuestion ${LANG_ITALIAN} \
  "Desiderate eliminare i vostri dati contabili?$\n(base di dati, configurazione, registri)"

LangString DataDeleted ${LANG_ENGLISH} "Data deleted: $APPDATA\LedgerAlps"
LangString DataDeleted ${LANG_FRENCH}  "Données supprimées : $APPDATA\LedgerAlps"
LangString DataDeleted ${LANG_GERMAN}  "Daten gelöscht: $APPDATA\LedgerAlps"
LangString DataDeleted ${LANG_ITALIAN} "Dati eliminati: $APPDATA\LedgerAlps"

LangString DataKept ${LANG_ENGLISH} "Your data in $APPDATA\LedgerAlps has been kept."
LangString DataKept ${LANG_FRENCH}  "Vos données dans $APPDATA\LedgerAlps ont été conservées."
LangString DataKept ${LANG_GERMAN}  "Ihre Daten in $APPDATA\LedgerAlps wurden beibehalten."
LangString DataKept ${LANG_ITALIAN} "I vostri dati in $APPDATA\LedgerAlps sono stati conservati."

LangString RunApp ${LANG_ENGLISH} "Launch LedgerAlps"
LangString RunApp ${LANG_FRENCH}  "Lancer LedgerAlps"
LangString RunApp ${LANG_GERMAN}  "LedgerAlps starten"
LangString RunApp ${LANG_ITALIAN} "Avviare LedgerAlps"

; Arrêt des composants. Ce que taskkill écrit sur sa console n'est PAS
; repris ici : voir la macro ArreterProcessus plus bas.
LangString StoppingApp ${LANG_ENGLISH} "Stopping LedgerAlps if it is running..."
LangString StoppingApp ${LANG_FRENCH}  "Arrêt de LedgerAlps s'il est en cours d'exécution…"
LangString StoppingApp ${LANG_GERMAN}  "LedgerAlps wird beendet, falls es läuft …"
LangString StoppingApp ${LANG_ITALIAN} "Arresto di LedgerAlps se è in esecuzione…"

LangString StopFailed ${LANG_ENGLISH} \
  "A component could not be stopped. Close LedgerAlps, then run this again."
LangString StopFailed ${LANG_FRENCH}  \
  "Un composant n'a pas pu être arrêté. Fermez LedgerAlps, puis relancez cette opération."
LangString StopFailed ${LANG_GERMAN}  \
  "Eine Komponente konnte nicht beendet werden. Schliessen Sie LedgerAlps und starten Sie diesen Vorgang erneut."
LangString StopFailed ${LANG_ITALIAN} \
  "Un componente non ha potuto essere arrestato. Chiudete LedgerAlps, poi rilanciate questa operazione."

LangString InstallDone1 ${LANG_ENGLISH} "Installation complete."
LangString InstallDone1 ${LANG_FRENCH}  "Installation terminée."
LangString InstallDone1 ${LANG_GERMAN}  "Installation abgeschlossen."
LangString InstallDone1 ${LANG_ITALIAN} "Installazione completata."

LangString InstallDone2 ${LANG_ENGLISH} \
  "Launch LedgerAlps from the Desktop or the Start Menu."
LangString InstallDone2 ${LANG_FRENCH}  \
  "Lancez LedgerAlps depuis le Bureau ou le menu Démarrer."
LangString InstallDone2 ${LANG_GERMAN}  \
  "Starten Sie LedgerAlps über den Desktop oder das Startmenü."
LangString InstallDone2 ${LANG_ITALIAN} \
  "Avviate LedgerAlps dal Desktop o dal menu Start."

LangString InstallDone3 ${LANG_ENGLISH} \
  "On first launch a setup wizard opens in your browser."
LangString InstallDone3 ${LANG_FRENCH}  \
  "Au premier lancement, un assistant de configuration s'ouvre dans votre navigateur."
LangString InstallDone3 ${LANG_GERMAN}  \
  "Beim ersten Start öffnet sich ein Einrichtungsassistent in Ihrem Browser."
LangString InstallDone3 ${LANG_ITALIAN} \
  "Al primo avvio, un assistente di configurazione si apre nel vostro browser."

; Info-bulle des raccourcis, et nom du raccourci de désinstallation.
LangString ShortcutTip ${LANG_ENGLISH} "Open LedgerAlps"
LangString ShortcutTip ${LANG_FRENCH}  "Ouvrir LedgerAlps"
LangString ShortcutTip ${LANG_GERMAN}  "LedgerAlps öffnen"
LangString ShortcutTip ${LANG_ITALIAN} "Aprire LedgerAlps"

LangString UninstallLink ${LANG_ENGLISH} "Uninstall LedgerAlps"
LangString UninstallLink ${LANG_FRENCH}  "Désinstaller LedgerAlps"
LangString UninstallLink ${LANG_GERMAN}  "LedgerAlps deinstallieren"
LangString UninstallLink ${LANG_ITALIAN} "Disinstallare LedgerAlps"

LangString UninstallDone ${LANG_ENGLISH} "LedgerAlps has been uninstalled."
LangString UninstallDone ${LANG_FRENCH}  "LedgerAlps a été désinstallé."
LangString UninstallDone ${LANG_GERMAN}  "LedgerAlps wurde deinstalliert."
LangString UninstallDone ${LANG_ITALIAN} "LedgerAlps è stato disinstallato."

; --------------------------------------------------------------------------- ;
; Arrêter un processus SANS recopier ce qu'il écrit sur sa console            ;
;                                                                             ;
; `nsExec::ExecToLog` reversait la sortie de `taskkill` dans le journal. Cette ;
; sortie est écrite dans la page de codes CONSOLE (CP850 sur un Windows        ;
; français), que NSIS relit comme de l'ANSI : « Opération réussie » devenait   ;
; « Op‚ration r,ussie », et l'espace insécable avant les deux-points sortait   ;
; en « ÿ ». Illisible, et pour rien.                                          ;
;                                                                             ;
; Transcoder ne suffirait pas à rendre ces lignes utiles. « ERREUR : le        ;
; processus est introuvable » est le cas NORMAL — l'application n'était pas    ;
; lancée — et s'afficher comme une erreur alarme sans raison. On ne garde donc ;
; que le code de retour, et on écrit nos propres phrases, traduites.          ;
;                                                                             ;
; Codes de `taskkill` : 0 = arrêté, 128 = aucun processus de ce nom. Les deux  ;
; sont des succès ici ; le reste (accès refusé, par exemple) mérite un mot.   ;
; --------------------------------------------------------------------------- ;
!macro ArreterProcessus exe
  nsExec::Exec 'taskkill /f /im "${exe}"'
  Pop $0
  StrCmp $0 "0" +3 0
  StrCmp $0 "128" +2 0
  DetailPrint "$(StopFailed)"
!macroend

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
; Pre-install: detect a reinstall                                              ;
; --------------------------------------------------------------------------- ;
; L'arrêt de l'application a QUITTÉ cette fonction.
;
; `.onInit` s'exécute au lancement de l'installeur, avant même la page de
; bienvenue : LedgerAlps était donc fermé de force chez quelqu'un qui n'avait
; encore rien accepté, et qui pouvait très bien annuler à la page de licence.
; L'arrêt est maintenant en tête de la section, une fois l'installation
; confirmée — et toujours avant l'écriture des fichiers, qui vient plus bas.
; Au passage, `DetailPrint` n'écrit nulle part depuis `.onInit` : la vue de
; détail n'existe pas encore.
Function .onInit
  ; Detect reinstall: if config.json already exists, write a sentinel so the
  ; launcher can show a "configuration preserved" notification on next launch.
  IfFileExists "$APPDATA\LedgerAlps\config.json" 0 lbl_no_reinstall
    FileOpen $0 "$APPDATA\LedgerAlps\.reinstalled" w
    FileClose $0
  lbl_no_reinstall:
FunctionEnd

; --------------------------------------------------------------------------- ;
; Installer section                                                           ;
; --------------------------------------------------------------------------- ;
Section "LedgerAlps (required)" SecMain
  SectionIn RO

  ; Arrêter le serveur et le lanceur pour que leurs fichiers soient
  ; remplaçables. Avant les commandes File ci-dessous, donc à temps.
  DetailPrint "$(StoppingApp)"
  !insertmacro ArreterProcessus "${SERVER_EXE}"
  !insertmacro ArreterProcessus "${LAUNCHER_EXE}"
  Sleep 1000

  SetOutPath "$INSTDIR"
  File "${LAUNCHER_EXE}"
  File "${SERVER_EXE}"
  File "${CLI_EXE}"
  File "..\..\LICENSE"

  ; Note: the React frontend is embedded inside ledgeralps-server.exe (Go embed).
  ; No separate dist\ folder is needed in the install directory.
  ;
  ; Builds before v1.1.1 shipped the frontend as loose files. Upgrading never
  ; removed them, so installations that go back that far still carry a stale
  ; index.html and assets\ that nothing reads — and, because the uninstaller
  ; used a non-recursive RMDir, they kept the install directory alive after an
  ; uninstall. Clearing them here retires the leftovers on the next upgrade.
  Delete "$INSTDIR\index.html"
  RMDir /r "$INSTDIR\assets"

  ; ── Shortcuts ──────────────────────────────────────────────────────────── ;
  ; Start Menu
  CreateDirectory "$SMPROGRAMS\${PRODUCT_NAME}"
  CreateShortcut "$SMPROGRAMS\${PRODUCT_NAME}\LedgerAlps.lnk" \
    "$INSTDIR\${LAUNCHER_EXE}" "" "$INSTDIR\${LAUNCHER_EXE}" 0 \
    SW_SHOWNORMAL "" "$(ShortcutTip)"
  ; Le raccourci de désinstallation portait un nom anglais. L'ancien est
  ; effacé explicitement : sans cela, une mise à jour laisserait les deux
  ; côte à côte dans le menu Démarrer.
  Delete "$SMPROGRAMS\${PRODUCT_NAME}\Uninstall LedgerAlps.lnk"
  CreateShortcut "$SMPROGRAMS\${PRODUCT_NAME}\$(UninstallLink).lnk" \
    "$INSTDIR\Uninstall.exe"

  ; Desktop shortcut
  CreateShortcut "$DESKTOP\LedgerAlps.lnk" \
    "$INSTDIR\${LAUNCHER_EXE}" "" "$INSTDIR\${LAUNCHER_EXE}" 0 \
    SW_SHOWNORMAL "" "$(ShortcutTip)"

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
  DetailPrint "$(InstallDone1)"
  DetailPrint "$(InstallDone2)"
  DetailPrint "$(InstallDone3)"
SectionEnd

; --------------------------------------------------------------------------- ;
; Uninstaller                                                                 ;
; --------------------------------------------------------------------------- ;
Section "Uninstall"
  ; Stop any running server
  DetailPrint "$(StoppingApp)"
  !insertmacro ArreterProcessus "${SERVER_EXE}"
  !insertmacro ArreterProcessus "${LAUNCHER_EXE}"
  Sleep 500

  ; ── Ask user whether to delete accounting data ─────────────────────────── ;
  MessageBox MB_YESNO|MB_ICONQUESTION|MB_DEFBUTTON2 \
    "$(DeleteDataQuestion)" \
    IDYES lbl_delete_data IDNO lbl_keep_data

  lbl_delete_data:
    RMDir /r "$APPDATA\LedgerAlps"
    DetailPrint "$(DataDeleted)"
    Goto lbl_done_data

  lbl_keep_data:
    DetailPrint "$(DataKept)"

  lbl_done_data:

  ; Remove installed files
  Delete "$INSTDIR\${LAUNCHER_EXE}"
  Delete "$INSTDIR\${SERVER_EXE}"
  Delete "$INSTDIR\${CLI_EXE}"
  Delete "$INSTDIR\LICENSE"

  ; Pre-v1.1.1 leftovers (see the installer section). RMDir below is deliberately
  ; non-recursive so it never deletes files a user put here themselves; without
  ; clearing these two first, it silently fails and the folder outlives the
  ; uninstall.
  Delete "$INSTDIR\index.html"
  RMDir /r "$INSTDIR\assets"

  Delete "$INSTDIR\Uninstall.exe"
  RMDir  "$INSTDIR"

  ; Remove shortcuts
  Delete "$SMPROGRAMS\${PRODUCT_NAME}\*.lnk"
  RMDir  "$SMPROGRAMS\${PRODUCT_NAME}"
  Delete "$DESKTOP\LedgerAlps.lnk"

  ; Remove uninstall registry key
  DeleteRegKey HKLM "${UNINSTALL_KEY}"

  DetailPrint ""
  DetailPrint "$(UninstallDone)"
SectionEnd
