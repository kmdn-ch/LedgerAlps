; LedgerAlps — Inno Setup Script
; Build: ISCC installer\ledgeralps.iss (from the repo root)
; Requires:
;   dist\ledgeralps.exe          (launcher, built with -H=windowsgui)
;   dist\ledgeralps-server.exe   (API + static file server)
;   frontend\dist\               (React build output)
;
; Output: installer\Output\LedgerAlps_Setup_<version>_windows_amd64.exe

#define AppName      "LedgerAlps"
#define AppVersion   "1.1.0"
#define AppPublisher "kmdn-ch"
#define AppURL       "https://github.com/kmdn-ch/LedgerAlps"
#define AppExe       "ledgeralps.exe"
#define ServerExe    "ledgeralps-server.exe"

[Setup]
AppId={{6F4A3B2C-1D0E-4F5A-8B9C-2D3E4F5A6B7C}
AppName={#AppName}
AppVersion={#AppVersion}
AppVerName={#AppName} {#AppVersion}
AppPublisher={#AppPublisher}
AppPublisherURL={#AppURL}
AppSupportURL={#AppURL}/issues
AppUpdatesURL={#AppURL}/releases
DefaultDirName={autopf}\{#AppName}
DefaultGroupName={#AppName}
DisableProgramGroupPage=yes
LicenseFile=..\LICENSE
OutputDir=Output
OutputBaseFilename=LedgerAlps_Setup_{#AppVersion}_windows_amd64
SetupIconFile=assets\icon.ico
Compression=lzma2/ultra64
SolidCompression=yes
WizardStyle=modern
WizardImageFile=assets\wizard-banner.bmp
WizardSmallImageFile=assets\wizard-small.bmp
PrivilegesRequired=admin
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
UninstallDisplayName={#AppName}
UninstallDisplayIcon={app}\{#AppExe}
; Disable autostart on next Windows login by default (user controls the launcher)
CloseApplications=yes

[Languages]
Name: "french";   MessagesFile: "compiler:Languages\French.isl"
Name: "english";  MessagesFile: "compiler:Default.isl"
Name: "german";   MessagesFile: "compiler:Languages\German.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"
Name: "startupentry"; Description: "Lancer LedgerAlps au démarrage de Windows"; GroupDescription: "Démarrage automatique"; Flags: unchecked

[Files]
; Launcher (main shortcut target — opens browser, starts server in background)
Source: "..\dist\{#AppExe}";    DestDir: "{app}"; Flags: ignoreversion

; Backend server
Source: "..\dist\{#ServerExe}"; DestDir: "{app}"; Flags: ignoreversion

; Frontend static files (served by the server from <install dir>\dist\)
Source: "..\frontend\dist\*";   DestDir: "{app}\dist"; Flags: ignoreversion recursesubdirs createallsubdirs

; Documentation
Source: "..\LICENSE";           DestDir: "{app}"; Flags: ignoreversion
Source: "..\README.md";         DestDir: "{app}"; Flags: ignoreversion

[Icons]
; Start Menu
Name: "{group}\{#AppName}";            Filename: "{app}\{#AppExe}"; Comment: "Ouvrir LedgerAlps"
Name: "{group}\Désinstaller {#AppName}"; Filename: "{uninstallexe}"

; Desktop
Name: "{autodesktop}\{#AppName}"; Filename: "{app}\{#AppExe}"; Tasks: desktopicon

; Startup (optional task)
Name: "{userstartup}\{#AppName}"; Filename: "{app}\{#AppExe}"; Tasks: startupentry

[Run]
; Launch the app after installation completes.
Filename: "{app}\{#AppExe}"; Description: "Lancer {#AppName} maintenant"; Flags: nowait postinstall skipifsilent

[UninstallDelete]
; Leave AppData config and database — user data must not be deleted on uninstall.
; If the user wants a clean uninstall they can delete %APPDATA%\LedgerAlps manually.

[Code]
// Stop any running server before upgrading files.
function InitializeSetup(): Boolean;
begin
  Result := True;
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssInstall then begin
    // Best-effort: kill any running server so files can be replaced.
    Exec('taskkill', '/f /im ledgeralps-server.exe', '', SW_HIDE, ewWaitUntilTerminated, 0);
    Exec('taskkill', '/f /im ledgeralps.exe',        '', SW_HIDE, ewWaitUntilTerminated, 0);
  end;
end;
