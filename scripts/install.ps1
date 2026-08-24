#Requires -Version 5.1
<#
.SYNOPSIS
    LedgerAlps Windows installer.

.DESCRIPTION
    Downloads and installs LedgerAlps on Windows.
    Installs ledgeralps-server.exe to C:\Program Files\LedgerAlps\,
    adds the directory to the system PATH, and registers a Windows Service.

.PARAMETER Version
    Specific version to install (e.g. "v1.2.3"). Defaults to latest release.

.PARAMETER InstallDir
    Installation directory. Default: C:\Program Files\LedgerAlps

.PARAMETER DataDir
    Data directory for config and SQLite DB. Default: C:\ProgramData\LedgerAlps

.PARAMETER NoService
    Skip Windows Service registration.

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
        "ARM64"  { return "arm64" }
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
    # C:\Program Files, puis a enregistrer en service a demarrage automatique,
    # ce que le reseau avait bien voulu rendre. Le mode d'emploi documente est
    # « irm ... | iex » : il n'y a aucune autre occasion d'inspecter.
    Write-Info "Verifying checksum..."
    $ligne = Select-String -Path $sumsPath -Pattern ([regex]::Escape($archive)) |
             Select-Object -First 1
    if (-not $ligne) {
        Remove-Item $tmpDir -Recurse -Force -ErrorAction SilentlyContinue
        Write-Fail "Aucune empreinte publiee pour $archive - installation abandonnee."
    }
    $attendu = ($ligne.Line -split '\s+')[0]
    $obtenu  = (Get-FileHash -Path $zipPath -Algorithm SHA256).Hash
    if ($obtenu -ne $attendu.ToUpper()) {
        Remove-Item $tmpDir -Recurse -Force -ErrorAction SilentlyContinue
        Write-Fail ("Empreinte SHA-256 incorrecte pour $archive.`n" +
                    "  attendue : $($attendu.ToUpper())`n" +
                    "  obtenue  : $obtenu`n" +
                    "  L'archive a ete modifiee ou le telechargement est corrompu. NE PAS INSTALLER.")
    }
    Write-Success "Checksum verified"

    Write-Info "Extracting to $InstallDir..."
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    Expand-Archive -Path $zipPath -DestinationPath $tmpDir -Force

    # Chemins ATTENDUS, pas une fouille recursive : -Recurse ramasserait le
    # premier fichier de ce nom trouve, quelle qu'en soit l'origine, pour aller
    # l'installer en service.
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
    $regKey  = "HKLM:\SYSTEM\CurrentControlSet\Control\Session Manager\Environment"
    $current = (Get-ItemProperty -Path $regKey -Name Path).Path

    if ($current -notlike "*$InstallDir*") {
        Write-Info "Adding $InstallDir to system PATH..."
        Set-ItemProperty -Path $regKey -Name Path -Value "$current;$InstallDir"
        # Broadcast environment change to running processes
        $HWND_BROADCAST = [IntPtr]0xffff
        $WM_WININICHANGE = 0x001A
        [System.Runtime.InteropServices.Marshal]::AllocHGlobal(0) | Out-Null
        Write-Success "PATH updated (new terminals will see the change)"
    } else {
        Write-Info "PATH already contains $InstallDir"
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
    # administrateurs ; le compte de service recoit son acces plus tard, a
    # l'enregistrement du service, et lui seul.
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
# Register Windows Service                                                    #
# --------------------------------------------------------------------------- #
function Register-Service {
    if ($NoService) {
        Write-Warn "Skipping Windows Service registration (--NoService)"
        return
    }

    $serviceName = "LedgerAlps"
    $exePath     = "$InstallDir\ledgeralps-server.exe"
    $envFile     = "$DataDir\ledgeralps.env"

    # Remove existing service if present
    $existing = Get-Service -Name $serviceName -ErrorAction SilentlyContinue
    if ($existing) {
        Write-Info "Stopping and removing existing service..."
        if ($existing.Status -eq "Running") {
            Stop-Service -Name $serviceName -Force
            Start-Sleep -Seconds 2
        }
        sc.exe delete $serviceName | Out-Null
        if ($LASTEXITCODE -ne 0) { Write-Fail "sc.exe delete a echoue (code $LASTEXITCODE)" }
        # Un service reste « marque pour suppression » tant qu'un handle est
        # ouvert dessus. Attendre sa disparition REELLE, plutot qu'une seconde
        # au hasard : sinon sc.exe create echoue avec 1072 et le script
        # annoncait quand meme « registered ».
        for ($i = 0; $i -lt 60; $i++) {
            if (-not (Get-Service -Name $serviceName -ErrorAction SilentlyContinue)) { break }
            Start-Sleep -Milliseconds 500
        }
        if (Get-Service -Name $serviceName -ErrorAction SilentlyContinue) {
            Write-Fail "Le service '$serviceName' est toujours marque pour suppression. Fermez services.msc et relancez."
        }
    }

    Write-Info "Registering Windows Service '$serviceName'..."

    # PAS de fichier .bat intermediaire.
    #
    # Le SCM attend que le processus qu'il lance appelle StartServiceCtrlDispatcher
    # dans le delai imparti. cmd.exe executant un script batch ne le fait jamais :
    # le service echouait donc au demarrage avec l'erreur 1053, systematiquement,
    # quel qu'ait ete le contenu du .bat. Celui-ci avait par ailleurs deux
    # defauts propres — « %%A:~0,1% » n'est pas une syntaxe valide sur une
    # metavariable for, donc le filtre de commentaires ne filtrait rien, et
    # set "%%A=%%B" sortait de ses guillemets sur une valeur contenant " et &.
    #
    # Le SCM sait charger un environnement lui-meme, via la valeur REG_MULTI_SZ
    # « Environment » sous la cle du service. Plus d'interpreteur au milieu.
    $ancien = "$InstallDir\start-service.bat"
    if (Test-Path $ancien) {
        Write-Info "Removing obsolete start-service.bat wrapper..."
        Remove-Item $ancien -Force
    }

    # Compte de service dedie, pas LocalSystem.
    #
    # Sans obj=, sc.exe retombe sur LocalSystem : un serveur HTTP servant une
    # base comptable tournerait avec les droits de la machine, et toute
    # execution de code dans le processus deviendrait un controle total du
    # poste. Le pendant Linux de ce service tourne deja sous un compte dedie
    # avec NoNewPrivileges et ProtectSystem=strict — le raisonnement de moindre
    # privilege avait ete fait une fois, puis pas porte sur Windows.
    #
    # « NT SERVICE\<nom> » est un compte de service virtuel : cree par le SCM a
    # l'enregistrement, sans mot de passe a stocker nulle part, et sans aucun
    # droit herite.
    $svcAccount = "NT SERVICE\$serviceName"

    sc.exe create $serviceName binPath= "`"$exePath`"" `
        DisplayName= "LedgerAlps Accounting Server" start= auto obj= $svcAccount | Out-Null
    if ($LASTEXITCODE -ne 0) { Write-Fail "sc.exe create a echoue (code $LASTEXITCODE)" }

    sc.exe description $serviceName "LedgerAlps Swiss SME Accounting - double-entry bookkeeping with QR-bill and ISO 20022 support." | Out-Null
    if ($LASTEXITCODE -ne 0) { Write-Warn "sc.exe description a echoue (code $LASTEXITCODE)" }

    sc.exe failure $serviceName reset= 60 actions= restart/5000/restart/10000/restart/30000 | Out-Null
    if ($LASTEXITCODE -ne 0) { Write-Warn "sc.exe failure a echoue (code $LASTEXITCODE)" }

    # L'environnement, lu par le SCM au demarrage. Seules les lignes NOM=VALEUR
    # sont retenues : les commentaires et les lignes vides du fichier .env
    # n'ont rien a faire ici, et c'est le filtre que le .bat croyait appliquer.
    if (Test-Path $envFile) {
        $envLines = @(Get-Content $envFile |
            Where-Object { $_ -match '^\s*[A-Za-z_][A-Za-z0-9_]*=' } |
            ForEach-Object { $_.Trim() })
        if ($envLines.Count -gt 0) {
            New-ItemProperty -Path "HKLM:\SYSTEM\CurrentControlSet\Services\$serviceName" `
                -Name Environment -PropertyType MultiString -Value $envLines -Force | Out-Null
            Write-Info "Service environment loaded from $envFile ($($envLines.Count) variables)"
        }
    } else {
        Write-Warn "$envFile absent : le service demarrera sans configuration. Creez-le, puis relancez cet installeur."
    }

    # Le compte de service virtuel n'herite d'aucun droit : lui accorder
    # explicitement l'acces a ses seules donnees, et rien d'autre.
    icacls $DataDir /grant "${svcAccount}:(OI)(CI)M" /T | Out-Null
    if ($LASTEXITCODE -ne 0) { Write-Warn "icacls sur $DataDir a echoue (code $LASTEXITCODE) - verifiez les droits du service" }

    Write-Success "Windows Service '$serviceName' registered (running as $svcAccount)"
    Write-Info "Start with: Start-Service -Name '$serviceName'"
    Write-Info "  or:       sc.exe start $serviceName"
    Write-Info "Note: apres modification de $envFile, relancez cet installeur pour recharger l'environnement du service."
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
    Write-Host "  1. Edit the config file:" -ForegroundColor White
    Write-Host "       Copy-Item '$DataDir\ledgeralps.env.example' '$DataDir\ledgeralps.env'" -ForegroundColor Gray
    Write-Host "       # Set JWT_SECRET to a strong random value (32+ characters)" -ForegroundColor Gray
    Write-Host ""
    Write-Host "  2. Start the service:" -ForegroundColor White
    Write-Host "       Start-Service -Name 'LedgerAlps'" -ForegroundColor Gray
    Write-Host ""
    Write-Host "  3. Create your admin user:" -ForegroundColor White
    Write-Host "       ledgeralps-cli bootstrap --email=admin@example.com --password=yourpassword" -ForegroundColor Gray
    Write-Host ""
    Write-Host "  4. Open http://localhost:8000" -ForegroundColor White
    Write-Host ""
}

# --------------------------------------------------------------------------- #
# Main                                                                        #
# --------------------------------------------------------------------------- #
Assert-Elevated

$arch   = Get-Arch
$tag    = Resolve-Version

Install-Binaries -Tag $tag -Arch $arch
Add-ToPath
Write-EnvTemplate
Register-Service
Write-NextSteps -Tag $tag
