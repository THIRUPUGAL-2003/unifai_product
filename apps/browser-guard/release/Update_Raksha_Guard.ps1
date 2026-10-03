# ==============================================================================
# Raksha Guard - One-Click Auto Updater
# Double-click this script to update Guard to the latest version from server
# ==============================================================================

$ErrorActionPreference = "Stop"

# ── Config (reads from raksha_guard_config.json next to this script) ──────────
$ScriptDir   = Split-Path -Parent $MyInvocation.MyCommand.Path
$ConfigFile  = Join-Path $ScriptDir "raksha_guard_config.json"
$BackendURL  = ""
$GuardSecret = ""

if (Test-Path $ConfigFile) {
    try {
        $cfg = Get-Content $ConfigFile -Raw | ConvertFrom-Json
        $BackendURL = $cfg.backend_url.TrimEnd("/")
        if ($cfg.PSObject.Properties.Name -contains "guard_secret") {
            $GuardSecret = [string]$cfg.guard_secret
        }
    } catch {}
}

if (-not $BackendURL) {
    $BackendURL = Read-Host "Enter server URL (e.g. https://unifai.yespanchi.com)"
    $BackendURL = $BackendURL.TrimEnd("/")
}

$AuthHeaders = @{
    "User-Agent" = "Raksha-Guard-Updater/ps1"
}
if ($GuardSecret -and $GuardSecret.Trim().Length -gt 0) {
    $AuthHeaders["X-Raksha-Guard-Key"] = $GuardSecret.Trim()
} else {
    Write-Host "WARNING: guard_secret missing in config — download may fail if server requires it." -ForegroundColor Yellow
}

Write-Host ""
Write-Host "============================================================" -ForegroundColor Cyan
Write-Host "  Raksha Guard Auto-Updater" -ForegroundColor Cyan
Write-Host "  Server: $BackendURL" -ForegroundColor Cyan
Write-Host "============================================================" -ForegroundColor Cyan
Write-Host ""

# ── Step 1: Check current version ────────────────────────────────────────────
$CurrentVersion = "unknown"
$VersionFile = Join-Path $ScriptDir "VERSION.txt"
if (Test-Path $VersionFile) {
    $CurrentVersion = (Get-Content $VersionFile -Raw).Trim()
}
Write-Host "Current version: $CurrentVersion" -ForegroundColor Yellow

# ── Step 2: Check server version ─────────────────────────────────────────────
Write-Host "Checking server for latest version..." -ForegroundColor Gray
try {
    # Check version via HTTP header from download endpoint (no separate version API needed)
    $HeadResp = Invoke-WebRequest -Uri "$BackendURL/api/browser-ai/setup/download-windows.zip" -Method Head -Headers $AuthHeaders -UseBasicParsing -TimeoutSec 10
    $ServerVersion = $HeadResp.Headers["X-Raksha-Guard-Version"]
    if (-not $ServerVersion) { $ServerVersion = "latest" }
    Write-Host "Server version : $ServerVersion" -ForegroundColor Green
} catch {
    Write-Host "Could not fetch server version - proceeding with download anyway." -ForegroundColor Yellow
    $ServerVersion = "latest"
}

if ($CurrentVersion -eq $ServerVersion -and $ServerVersion -ne "unknown" -and $ServerVersion -ne "latest") {
    Write-Host ""
    Write-Host "Already up to date! ($CurrentVersion)" -ForegroundColor Green
    Write-Host "Press any key to exit..."
    $null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
    exit 0
}

# ── Step 3: Stop running Guard ────────────────────────────────────────────────
Write-Host ""
Write-Host "Stopping Raksha Guard..." -ForegroundColor Yellow
Get-Process -Name "Raksha_Guard" -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep -Seconds 2

# ── Step 4: Download latest installer (Windows ZIP) ───────────────────────────
$TempZip   = Join-Path $env:TEMP "Raksha_Guard_Update.zip"
$TempSetup = Join-Path $env:TEMP "Raksha_Guard_Update_Setup.exe"
Write-Host "Downloading latest installer from server..." -ForegroundColor Gray

try {
    Invoke-WebRequest `
        -Uri "$BackendURL/api/browser-ai/setup/download-windows.zip" `
        -OutFile $TempZip `
        -Headers $AuthHeaders `
        -UseBasicParsing `
        -TimeoutSec 120
    Write-Host "Download complete! Extracting..." -ForegroundColor Green
    # Extract Setup EXE from ZIP
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $zip = [System.IO.Compression.ZipFile]::OpenRead($TempZip)
    $entry = $zip.Entries | Where-Object { $_.Name -eq "Raksha_Guard_Setup.exe" } | Select-Object -First 1
    if ($entry) {
        [System.IO.Compression.ZipFileExtensions]::ExtractToFile($entry, $TempSetup, $true)
        Write-Host "Extracted Setup EXE." -ForegroundColor Green
    } else {
        throw "Raksha_Guard_Setup.exe not found in downloaded ZIP"
    }
    $zip.Dispose()
    Remove-Item $TempZip -ErrorAction SilentlyContinue
} catch {
    Write-Host "ERROR: Download failed - $_" -ForegroundColor Red
    Write-Host "Please download manually from: $BackendURL" -ForegroundColor Yellow
    Write-Host "Press any key to exit..."
    $null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
    exit 1
}

# ── Step 5: Install silently ───────────────────────────────────────────────────
Write-Host ""
Write-Host "Installing (silent)..." -ForegroundColor Yellow
try {
    Start-Process -FilePath $TempSetup -ArgumentList "/VERYSILENT /NORESTART /SUPPRESSMSGBOXES" -Wait
    Write-Host "Installation complete!" -ForegroundColor Green
} catch {
    Write-Host "ERROR: Install failed - $_" -ForegroundColor Red
    Write-Host "Try running manually: $TempSetup"
    Write-Host "Press any key to exit..."
    $null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
    exit 1
}

# ── Step 6: Cleanup ───────────────────────────────────────────────────────────
Remove-Item $TempSetup -ErrorAction SilentlyContinue

# ── Step 7: Restart Guard ─────────────────────────────────────────────────────
Write-Host ""
Write-Host "Starting Raksha Guard..." -ForegroundColor Yellow
$GuardExe = @(
    "$env:LOCALAPPDATA\Programs\Raksha\Guard\Raksha_Guard.exe",
    "$env:PROGRAMFILES\Raksha\Guard\Raksha_Guard.exe"
) | Where-Object { Test-Path $_ } | Select-Object -First 1

if ($GuardExe) {
    Start-Process $GuardExe
    Write-Host "Guard started!" -ForegroundColor Green
} else {
    Write-Host "Guard EXE not found in default location — please start manually." -ForegroundColor Yellow
}

Write-Host ""
Write-Host "============================================================" -ForegroundColor Cyan
Write-Host "  Update Complete! $CurrentVersion → $ServerVersion" -ForegroundColor Green
Write-Host "============================================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "Press any key to exit..."
$null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
