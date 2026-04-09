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

    $archive   = "ledgeralps_${Tag}_windows_${Arch}.zip"
    $url       = "https://github.com/$Repo/releases/download/$Tag/$archive"
    $tmpDir    = [System.IO.Path]::Combine([System.IO.Path]::GetTempPath(), "ledgeralps-install")
    $zipPath   = [System.IO.Path]::Combine($tmpDir, $archive)

    New-Item -ItemType Directory -Path $tmpDir -Force | Out-Null

    Write-Info "Downloading $archive..."
    try {
        $ProgressPreference = 'SilentlyContinue'
        Invoke-WebRequest -Uri $url -OutFile $zipPath -UseBasicParsing
    } catch {
        Write-Fail "Download failed: $_`n  URL: $url"
    }

    Write-Info "Extracting to $InstallDir..."
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    Expand-Archive -Path $zipPath -DestinationPath $tmpDir -Force

    $serverExe = Get-ChildItem -Path $tmpDir -Recurse -Filter "ledgeralps-server.exe" | Select-Object -First 1
    $cliExe    = Get-ChildItem -Path $tmpDir -Recurse -Filter "ledgeralps-cli.exe"    | Select-Object -First 1

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
        Start-Sleep -Seconds 1
    }

    Write-Info "Registering Windows Service '$serviceName'..."

    # sc.exe does not support environment variables natively.
    # We wrap the server in a batch file that loads the env file first.
    $wrapperBat = "$InstallDir\start-service.bat"
    @"
@echo off
for /f "usebackq tokens=1* delims==" %%A in ("$envFile") do (
    if not "%%A"=="" if not "%%A:~0,1%"=="#" set "%%A=%%B"
)
"$exePath"
"@ | Set-Content -Path $wrapperBat -Encoding ASCII

    $binPath = "`"$wrapperBat`""
    sc.exe create $serviceName binPath= $binPath DisplayName= "LedgerAlps Accounting Server" start= auto | Out-Null
    sc.exe description $serviceName "LedgerAlps Swiss SME Accounting — double-entry bookkeeping with QR-bill and ISO 20022 support." | Out-Null
    sc.exe failure $serviceName reset= 60 actions= restart/5000/restart/10000/restart/30000 | Out-Null

    Write-Success "Windows Service '$serviceName' registered"
    Write-Info "Start with: Start-Service -Name '$serviceName'"
    Write-Info "  or:       sc.exe start $serviceName"
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
