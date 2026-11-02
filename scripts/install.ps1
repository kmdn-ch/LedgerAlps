#Requires -Version 5.1
<#
.SYNOPSIS
    LedgerAlps Windows installer.

.DESCRIPTION
    Downloads and installs LedgerAlps on Windows.
    Installs ledgeralps-server.exe and ledgeralps-cli.exe to C:\Program Files\LedgerAlps\,
    adds the directory to the system PATH and writes a configuration template.

    This script does NOT register a Windows Service. ledgeralps-server.exe does not speak
    the Service Control Manager protocol, so a service pointing at it could never start
    (error 1053). A service left by an earlier run of this script is stopped and removed.
    For a desktop installation use the Windows installer (LedgerAlps_Setup_*.exe).

.PARAMETER Version
    Specific version to install (e.g. "v1.2.3"). Defaults to latest release.

.PARAMETER InstallDir
    Installation directory. Default: C:\Program Files\LedgerAlps

.PARAMETER DataDir
    Data directory for config and SQLite DB. Default: C:\ProgramData\LedgerAlps

.PARAMETER NoService
    Obsolete and ignored: this script no longer registers a service. Kept so that
    existing command lines keep working.

.EXAMPLE
    # Latest version
    irm https://raw.githubusercontent.com/kmdn-ch/ledgeralps/main/scripts/install.ps1 | iex

    # Specific version
    & { $Version = "v1.2.3"; irm https://raw.githubusercontent.com/kmdn-ch/ledgeralps/main/scripts/install.ps1 | iex }
#>
[CmdletBinding()]
param(
    [string]$Version    = $env:LEDGERALPS_VERSION,
    [string]$InstallDir = "C:\Program Files\LedgerAlps",
    [string]$DataDir    = "C:\ProgramData\LedgerAlps",
    [switch]$NoService
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$Repo = "kmdn-ch/ledgeralps"

function Write-Info    { param($Msg) Write-Host "[ledgeralps] $Msg" -ForegroundColor Cyan }
function Write-Success { param($Msg) Write-Host "[ledgeralps] $Msg" -ForegroundColor Green }
function Write-Warn    { param($Msg) Write-Host "[ledgeralps] WARN: $Msg" -ForegroundColor Yellow }
function Write-Fail    { param($Msg) Write-Error "[ledgeralps] ERROR: $Msg" }

# Assert-CheminSain est le pendant Windows de valider_chemin
# (scripts/install.sh:31), qui manquait ici.
#
# $InstallDir finit dans le PATH MACHINE (voir Add-ToPath) : un « ; »
# y injecte des entrees supplementaires pour TOUS les comptes. $DataDir
# finit en argument d icacls et dans le gabarit .env. Le prerequis est
# d etre deja administrateur, donc ce n est pas une elevation -- c est
# une asymetrie avec le raisonnement deja tenu cote Linux, et le geste
# coute six lignes.
function Assert-CheminSain {
    param([string]$Nom, [string]$Chemin)
    if ([string]::IsNullOrWhiteSpace($Chemin)) {
        Write-Fail "$Nom ne peut pas etre vide."
    }
    if ($Chemin -match '[;"|<>*?]' -or $Chemin -match '[\r\n]') {
        Write-Fail "$Nom contient un caractere refuse (; `" | < > * ? ou saut de ligne) : $Chemin"
    }
    if (-not [System.IO.Path]::IsPathRooted($Chemin)) {
        Write-Fail "$Nom doit etre un chemin absolu (recu : $Chemin)"
    }
}

# Assert-DossierDeDonnees refuse un dossier de donnees PREEXISTANT qui n'est pas digne de confiance
# (audit croise Gemini, HIGH-25).
#
# C:\ProgramData laisse tout utilisateur local CREER un sous-dossier. Quelqu'un qui cree
# C:\ProgramData\LedgerAlps avant l'installation en reste le PROPRIETAIRE -- et un proprietaire
# garde WRITE_DAC, quoi que l'installeur fasse ensuite de l'heritage et des droits : il se rend
# l'acces quand il veut, et lit la cle qui signe les sessions et la comptabilite entiere. Une
# jonction deposee au meme endroit ferait suivre a icacls un chemin choisi par lui.
#
# Le dossier est donc accepte s'il n'existe pas, ou s'il n'est ni un lien ni une jonction ET
# appartient a SYSTEM, au groupe Administrateurs ou au compte qui lance l'installation. Sinon
# l'installation s'arrete en disant quoi faire : l'administrateur regarde le dossier, puis le
# supprime ou en reprend la propriete en connaissance de cause.
#
# $SidsDeConfiance ne sert qu'aux essais.
function Assert-DossierDeDonnees {
    param(
        [string]$Chemin,
        [string[]]$SidsDeConfiance = @(
            'S-1-5-18',                                               # SYSTEM
            'S-1-5-32-544',                                           # Administrateurs
            [Security.Principal.WindowsIdentity]::GetCurrent().User.Value  # qui installe
        )
    )
    if (-not (Test-Path -LiteralPath $Chemin)) { return }

    $item = Get-Item -LiteralPath $Chemin -Force
    if ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) {
        Write-Fail ("$Chemin est une jonction ou un lien symbolique : l'installation ne le suit pas. " +
                    "Examinez-le, supprimez-le s'il n'a pas de raison d'etre, puis relancez.")
    }
    $proprio = (Get-Acl -LiteralPath $Chemin).Owner
    try {
        $sid = (New-Object Security.Principal.NTAccount($proprio)).Translate([Security.Principal.SecurityIdentifier]).Value
    } catch {
        Write-Fail "Le proprietaire de $Chemin ($proprio) n'a pas pu etre identifie : installation abandonnee."
    }
    if ($sid -notin $SidsDeConfiance) {
        Write-Fail ("$Chemin existe deja et appartient a $proprio, ni SYSTEM, ni les Administrateurs, ni le compte " +
                    "qui installe : son proprietaire en garderait le controle des droits, et lirait la cle de session " +
                    "et la comptabilite. Supprimez-le s'il a ete cree par un autre compte, ou reprenez-en la " +
                    "propriete (icacls `"$Chemin`" /setowner *S-1-5-32-544) apres l'avoir examine, puis relancez.")
    }
}

# --------------------------------------------------------------------------- #
# Elevation check                                                             #
# --------------------------------------------------------------------------- #
function Assert-Elevated {
    $principal = [Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()
    if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
        Write-Fail "This script must be run as Administrator. Re-run in an elevated PowerShell prompt."
    }
}

# --------------------------------------------------------------------------- #
# Detect architecture                                                         #
# --------------------------------------------------------------------------- #
function Get-Arch {
    switch ($env:PROCESSOR_ARCHITECTURE) {
        "AMD64"  { return "amd64" }
        # Aucun binaire windows/arm64 n'est publie (.goreleaser.yaml). Refuser
        # ICI avec la raison, plutot que de laisser un 404 se presenter comme
        # un echec de telechargement. Meme refus que install.sh, qui le dit
        # deja pour Linux.
        "ARM64"  { Write-Fail "Les binaires ARM ne sont plus publies depuis la v1.4.1. Utilisez l'installeur x86-64 (LedgerAlps_Setup_*_windows_amd64.exe) : Windows l'execute par emulation." }
        default  { Write-Fail "Unsupported architecture: $env:PROCESSOR_ARCHITECTURE" }
    }
}

# --------------------------------------------------------------------------- #
# Resolve latest release version                                              #
# --------------------------------------------------------------------------- #
function Resolve-Version {
    if ($Version) {
        Write-Info "Using specified version: $Version"
        return $Version
    }

    Write-Info "Fetching latest release version from GitHub..."
    $apiUrl = "https://api.github.com/repos/$Repo/releases/latest"
    try {
        $response = Invoke-RestMethod -Uri $apiUrl -UseBasicParsing
        $tag = $response.tag_name
        Write-Info "Latest version: $tag"
        return $tag
    } catch {
        Write-Fail "Could not fetch latest version from GitHub. Set -Version manually."
    }
}

# --------------------------------------------------------------------------- #
# Download and install binaries                                               #
# --------------------------------------------------------------------------- #
function Install-Binaries {
    param([string]$Tag, [string]$Arch)

    # Le tag porte un « v », le nom d'archive produit par GoReleaser non ; le
    # chemin de l'URL, lui, veut le tag entier. Utiliser $Tag pour les deux
    # demandait un fichier jamais publie, et l'echec sortait en
    # « Download failed » sans jamais nommer la cause. Voir release.yml:165.
    $versionNum = $Tag -replace '^v', ''
    $archive   = "ledgeralps_${versionNum}_windows_${Arch}.zip"
    $url       = "https://github.com/$Repo/releases/download/$Tag/$archive"
    $sums      = "ledgeralps_${versionNum}_checksums.txt"
    $sumsUrl   = "https://github.com/$Repo/releases/download/$Tag/$sums"

    # Nom NEUF a chaque execution, et pas de -Force.
    #
    # Un nom fixe reutilise par -Force accepte ce qu'un compte de moindre
    # privilege a prepare : dossier aux ACL ouvertes, ou jonction que le
    # Remove-Item final suivrait. Cela compte double quand ce script tourne en
    # SYSTEM (Intune, SCCM, agent de parc), car GetTempPath() vaut alors
    # C:\Windows\Temp, ou le groupe Users peut creer des dossiers.
    $tmpDir    = [System.IO.Path]::Combine([System.IO.Path]::GetTempPath(),
                   "ledgeralps-install-" + [System.Guid]::NewGuid().ToString('N'))
    $zipPath   = [System.IO.Path]::Combine($tmpDir, $archive)
    $sumsPath  = [System.IO.Path]::Combine($tmpDir, $sums)

    New-Item -ItemType Directory -Path $tmpDir -ErrorAction Stop | Out-Null

    Write-Info "Downloading $archive..."
    try {
        $ProgressPreference = 'SilentlyContinue'
        Invoke-WebRequest -Uri $url     -OutFile $zipPath  -UseBasicParsing
        Invoke-WebRequest -Uri $sumsUrl -OutFile $sumsPath -UseBasicParsing
    } catch {
        Write-Fail "Download failed: $_`n  URL: $url"
    }

    # Verifier l'empreinte AVANT d'extraire et d'installer.
    #
    # GoReleaser publie deja ces empreintes. Les ignorer revenait a copier dans
    # C:\Program Files, puis a laisser sur le PATH de tous les comptes,
    # ce que le reseau avait bien voulu rendre. Le mode d'emploi documente est
    # « irm ... | iex » : il n'y a aucune autre occasion d'inspecter.
    Write-Info "Verifying checksum..."
    # Correspondance sur le CHAMP, pas sur la ligne.
    #
    # Select-String retenait toute ligne CONTENANT le nom de l archive, et
    # « ...zip.sbom.json » -- publie par le bloc sboms de .goreleaser.yaml --
    # contient « ...zip ». Que la bonne ligne arrive en premier tenait au tri
    # de GoReleaser, que rien ne garantit ni ne documente : un futur artefact
    # au nom prefixe (attestation, .sig, .pem) reveillait le defaut.
    #
    # install.sh:156 fait deja la comparaison de champ avec awk ; on aligne.
    $attendu = Get-Content $sumsPath | ForEach-Object {
        $champs = $_ -split '\s+', 2
        if ($champs.Count -eq 2 -and $champs[1].Trim().TrimStart([char]42) -eq $archive) {
            $champs[0]
        }
    } | Select-Object -First 1
    if (-not $attendu) {
        Remove-Item $tmpDir -Recurse -Force -ErrorAction SilentlyContinue
        Write-Fail "Aucune empreinte publiee pour $archive - installation abandonnee."
    }
    $obtenu  = (Get-FileHash -Path $zipPath -Algorithm SHA256).Hash
    if ($obtenu -ne $attendu.ToUpper()) {
        Remove-Item $tmpDir -Recurse -Force -ErrorAction SilentlyContinue
        Write-Fail ("Empreinte SHA-256 incorrecte pour $archive.`n" +
                    "  attendue : $($attendu.ToUpper())`n" +
                    "  obtenue  : $obtenu`n" +
                    "  L'archive a ete modifiee ou le telechargement est corrompu. NE PAS INSTALLER.")
    }
    Write-Success "Checksum verified"

    # Les binaires ne se remplacent pas tant que le serveur tourne : un service laisse par une
    # execution anterieure de ce script est retire AVANT la copie (audit croise Gemini, HIGH-25 :
    # la copie echouait « fichier utilise par un autre processus »), et un serveur lance a la main
    # ou par le lanceur est nomme, pas tue.
    Remove-ObsoleteService
    Assert-AucunServeurEnCours

    Write-Info "Extracting to $InstallDir..."
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    Expand-Archive -Path $zipPath -DestinationPath $tmpDir -Force

    # Chemins ATTENDUS, pas une fouille recursive : -Recurse ramasserait le
    # premier fichier de ce nom trouve, quelle qu'en soit l'origine, pour aller
    # l'installer dans C:\Program Files.
    $serverExe = Get-Item (Join-Path $tmpDir "ledgeralps-server.exe") -ErrorAction SilentlyContinue
    $cliExe    = Get-Item (Join-Path $tmpDir "ledgeralps-cli.exe")    -ErrorAction SilentlyContinue

    if (-not $serverExe) { Write-Fail "ledgeralps-server.exe not found in archive." }
    if (-not $cliExe)    { Write-Fail "ledgeralps-cli.exe not found in archive." }

    Copy-Item $serverExe.FullName -Destination "$InstallDir\ledgeralps-server.exe" -Force
    Copy-Item $cliExe.FullName    -Destination "$InstallDir\ledgeralps-cli.exe"    -Force

    Remove-Item $tmpDir -Recurse -Force
    Write-Success "Installed binaries to $InstallDir"
}

# --------------------------------------------------------------------------- #
# Add install directory to system PATH                                        #
# --------------------------------------------------------------------------- #
function Add-ToPath {
    # Get-ItemProperty DEVELOPPE les REG_EXPAND_SZ, et Set-ItemProperty
    # reecrirait la forme developpee : %SystemRoot% disparaitrait du PATH
    # systeme, pour TOUTES les entrees, definitivement. Sur un poste ou
    # %SystemRoot% n'est pas C:\Windows -- image de deploiement, machine
    # virtuelle, changement de lettre de volume -- le PATH pointerait alors sur
    # des chemins morts : une panne a l'echelle de la machine, causee par un
    # installeur applicatif.
    $key = [Microsoft.Win32.Registry]::LocalMachine.OpenSubKey(
             'SYSTEM\CurrentControlSet\Control\Session Manager\Environment', $true)
    if (-not $key) { Write-Fail "Impossible d'ouvrir la cle Environment du registre." }
    try {
        $current = $key.GetValue('Path', '',
            [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)

        # Comparaison litterale entree par entree : -like traiterait [ ] * ?
        # d'un -InstallDir fourni par l'utilisateur comme des jokers.
        if ($InstallDir -notin ($current -split ';')) {
            Write-Info "Adding $InstallDir to system PATH..."
            $key.SetValue('Path', "$current;$InstallDir",
                [Microsoft.Win32.RegistryValueKind]::ExpandString)
            # Pas de diffusion WM_WININICHANGE : les trois lignes qui
            # pretendaient la faire n'appelaient aucun SendMessageTimeout et
            # ne faisaient qu'allouer un tampon natif de zero octet, jamais
            # libere. Un commentaire qui decrit une garantie inexistante est
            # pire que l'absence de garantie -- on dit donc ce qui est vrai.
            Write-Success "PATH updated (new terminals will see the change)"
        } else {
            Write-Info "PATH already contains $InstallDir"
        }
    } finally {
        $key.Dispose()
    }
}

# --------------------------------------------------------------------------- #
# Write env template                                                          #
# --------------------------------------------------------------------------- #
function Write-EnvTemplate {
    New-Item -ItemType Directory -Path $DataDir -Force | Out-Null

    # Ce repertoire recoit ledgeralps.env - donc JWT_SECRET, la cle qui SIGNE
    # les jetons de session - et la base SQLite, donc la comptabilite entiere.
    #
    # Les ACL heritees de C:\ProgramData donnent la lecture au groupe Users :
    # tout compte local de la machine lisait la cle et pouvait forger un jeton
    # administrateur. On coupe l'heritage et on ne laisse que SYSTEM et les
    # administrateurs. Le compte qui lancera le serveur a la main doit en recevoir l'acces
    # explicitement (voir Write-NextSteps) : il n'y a plus de compte de service qui l'aurait.
    icacls $DataDir /inheritance:r `
        /grant "*S-1-5-18:(OI)(CI)F" `
        /grant "*S-1-5-32-544:(OI)(CI)F" | Out-Null
    if ($LASTEXITCODE -ne 0) {
        Write-Fail "Impossible de restreindre les ACL de $DataDir (code $LASTEXITCODE). JWT_SECRET y serait lisible par tout compte local."
    }

    $envExample = "$DataDir\ledgeralps.env.example"
    if (-not (Test-Path $envExample)) {
        @"
# LedgerAlps environment configuration
# Copy this file to ledgeralps.env and fill in the values.

# REQUIRED: Generate a strong secret (32+ chars)
JWT_SECRET=CHANGE_ME_TO_A_32_CHAR_MINIMUM_SECRET

# HTTP port (default: 8000)
PORT=8000

# SQLite database path
SQLITE_PATH=$DataDir\ledgeralps.db

# OR use PostgreSQL (comment out SQLITE_PATH and uncomment below)
# POSTGRES_DSN=postgres://user:password@localhost:5432/ledgeralps?sslmode=disable

# CORS — allowed frontend origins (comma-separated)
ALLOWED_ORIGINS=http://localhost:5173,http://localhost:3000

# Logging
LOG_LEVEL=INFO
DEBUG=false
"@ | Set-Content -Path $envExample -Encoding UTF8
        Write-Info "Created env template at $envExample"
    }
}

# --------------------------------------------------------------------------- #
# Remove the service an earlier version of this script registered              #
# --------------------------------------------------------------------------- #
#
# Ce script enregistrait un service Windows (« LedgerAlps », NT SERVICE\LedgerAlps). Il ne pouvait
# pas demarrer : ledgeralps-server.exe n'appelle jamais StartServiceCtrlDispatcher, donc le
# Gestionnaire de services attendait en vain et abandonnait avec l'erreur 1053 (audit croise Gemini,
# CRIT-15, decision du proprietaire : retirer ce chemin). Le laisser en place donnerait un service
# « demarrage automatique » qui echoue a chaque demarrage de la machine.
#
# Il n'est retire que s'il POINTE sur notre binaire : un service du meme nom qui serait autre chose
# n'est pas le notre a supprimer.
function Remove-ObsoleteService {
    $serviceName = "LedgerAlps"
    $existing = Get-Service -Name $serviceName -ErrorAction SilentlyContinue
    if (-not $existing) { return }

    $cle = "HKLM:\SYSTEM\CurrentControlSet\Services\$serviceName"
    $chemin = $null
    try { $chemin = (Get-ItemProperty -Path $cle -Name ImagePath -ErrorAction Stop).ImagePath } catch { }
    if (-not $chemin -or $chemin -notlike "*ledgeralps-server.exe*") {
        Write-Warn "Un service '$serviceName' existe mais ne pointe pas sur ledgeralps-server.exe ($chemin) : laisse tel quel."
        return
    }

    Write-Info "Removing the Windows service registered by an earlier version of this script (it could never start)..."
    if ($existing.Status -ne "Stopped") {
        Stop-Service -Name $serviceName -Force -ErrorAction SilentlyContinue
        Start-Sleep -Seconds 2
    }
    sc.exe delete $serviceName | Out-Null
    if ($LASTEXITCODE -ne 0) { Write-Fail "sc.exe delete a echoue (code $LASTEXITCODE)" }
    # Un service reste « marque pour suppression » tant qu'un handle est ouvert dessus : attendre
    # sa disparition REELLE plutot qu'une seconde au hasard.
    for ($i = 0; $i -lt 60; $i++) {
        if (-not (Get-Service -Name $serviceName -ErrorAction SilentlyContinue)) { break }
        Start-Sleep -Milliseconds 500
    }
    if (Get-Service -Name $serviceName -ErrorAction SilentlyContinue) {
        Write-Fail "Le service '$serviceName' est toujours marque pour suppression. Fermez services.msc et relancez."
    }
    $ancien = "$InstallDir\start-service.bat"
    if (Test-Path $ancien) { Remove-Item $ancien -Force }
    Write-Success "Obsolete service removed"
}

# Un serveur en marche tient son binaire : la copie echouerait sur « fichier utilise par un autre
# processus ». On le NOMME au lieu de le tuer — c'est peut-etre une comptabilite ouverte.
function Assert-AucunServeurEnCours {
    $cible = Join-Path $InstallDir "ledgeralps-server.exe"
    $enCours = @(Get-Process -Name "ledgeralps-server" -ErrorAction SilentlyContinue |
        Where-Object { $_.Path -eq $cible })
    if ($enCours.Count -gt 0) {
        Write-Fail ("ledgeralps-server.exe tourne (PID $($enCours[0].Id)) depuis $InstallDir : fermez LedgerAlps " +
                    "(icone de la barre des taches, ou Stop-Process -Id $($enCours[0].Id)) puis relancez.")
    }
}

# --------------------------------------------------------------------------- #
# Print next steps                                                            #
# --------------------------------------------------------------------------- #
function Write-NextSteps {
    param([string]$Tag)

    Write-Host ""
    Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Green
    Write-Success "LedgerAlps $Tag installed successfully!"
    Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Green
    Write-Host ""
    Write-Host "  NEXT STEPS:" -ForegroundColor White
    Write-Host ""
    Write-Host "  This script installs the programs only: it registers NO Windows service." -ForegroundColor Yellow
    Write-Host "  For a desktop installation (launcher, setup wizard), use LedgerAlps_Setup_*.exe instead." -ForegroundColor Yellow
    Write-Host ""
    Write-Host "  1. Edit the config file:" -ForegroundColor White
    Write-Host "       Copy-Item '$DataDir\ledgeralps.env.example' '$DataDir\ledgeralps.env'" -ForegroundColor Gray
    Write-Host "       # Set JWT_SECRET to a strong random value (32+ characters)" -ForegroundColor Gray
    Write-Host ""
    Write-Host "  2. Let the account that will run the server read and write the data folder" -ForegroundColor White
    Write-Host "     (it is restricted to SYSTEM and Administrators):" -ForegroundColor White
    Write-Host "       icacls '$DataDir' /grant `"${env:USERNAME}:(OI)(CI)M`"" -ForegroundColor Gray
    Write-Host ""
    Write-Host "  3. Run the server in a terminal, with that file's settings:" -ForegroundColor White
    Write-Host "       Get-Content '$DataDir\ledgeralps.env' | Where-Object { `$_ -match '^\s*[A-Za-z_][A-Za-z0-9_]*=' } |" -ForegroundColor Gray
    Write-Host "         ForEach-Object { `$k,`$v = `$_ -split '=',2; Set-Item `"env:`$(`$k.Trim())`" `$v.Trim() }" -ForegroundColor Gray
    Write-Host "       ledgeralps-server" -ForegroundColor Gray
    Write-Host "     To keep it running without a session, use a supervisor of your choice (Task Scheduler," -ForegroundColor Gray
    Write-Host "     NSSM...) around that command." -ForegroundColor Gray
    Write-Host ""
    Write-Host "  4. Create your admin user:" -ForegroundColor White
    Write-Host "       ledgeralps-cli bootstrap --email=admin@example.com --password=yourpassword" -ForegroundColor Gray
    Write-Host ""
    Write-Host "  5. Open http://localhost:8000" -ForegroundColor White
    Write-Host ""
}

# --------------------------------------------------------------------------- #
# Main                                                                        #
# --------------------------------------------------------------------------- #
Assert-Elevated
Assert-CheminSain -Nom 'InstallDir' -Chemin $InstallDir
Assert-CheminSain -Nom 'DataDir'    -Chemin $DataDir
Assert-DossierDeDonnees -Chemin $DataDir

$arch   = Get-Arch
$tag    = Resolve-Version

Install-Binaries -Tag $tag -Arch $arch
Add-ToPath
Write-EnvTemplate
if ($NoService) { Write-Warn "-NoService is obsolete and ignored: this script no longer registers a service." }
Write-NextSteps -Tag $tag
