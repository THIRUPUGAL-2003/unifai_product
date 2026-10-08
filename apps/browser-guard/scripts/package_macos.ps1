Set-Location -Path $PSScriptRoot\..

# Sync fleet config + live proxy scripts into the shipped .app (Windows host can do this;
# full Darwin binary rebuild still requires make build-guard-mac on a Mac).
if (-not (Test-Path 'release\Gateway_Guard.app')) {
    Write-Error "release\Gateway_Guard.app missing - copy a Mac-built .app first (make build-guard-mac)."
    exit 1
}

Copy-Item -Force 'release\gateway_guard_config.json' 'release\Gateway_Guard.app\Contents\Resources\gateway_guard_config.json'
Copy-Item -Force 'proxy\browser_ai_proxy.py' 'release\Gateway_Guard.app\Contents\Resources\browser_ai_proxy.py'
$partsDest = 'release\Gateway_Guard.app\Contents\Resources\gateway_proxy_parts'
New-Item -ItemType Directory -Force -Path $partsDest | Out-Null
if (-not (Test-Path 'proxy\gateway_proxy_parts\gateway_proxy_parts.enc')) {
    Write-Error "proxy\gateway_proxy_parts\gateway_proxy_parts.enc missing. Run installer\encrypt_proxy_bundle.py before packaging."
    exit 1
}
Get-ChildItem $partsDest -File -ErrorAction SilentlyContinue | Where-Object { $_.Extension -eq '.py' -or $_.Name -eq 'README.md' -or $_.Name -eq 'Gateway_proxy_parts.enc' } | Remove-Item -Force -ErrorAction SilentlyContinue
Copy-Item -Force 'proxy\gateway_proxy_parts\gateway_proxy_parts.enc' (Join-Path $partsDest 'gateway_proxy_parts.enc')
Copy-Item -Force 'proxy\gateway_proxy_parts\MANIFEST.txt' (Join-Path $partsDest 'MANIFEST.txt')
# This .app was built on a Mac before the decryptor was compiled into the binary.
# Ship the scrambled decryptor so the existing Mac binary can open the enc.
# A later make build-guard-mac on a Mac leaves this file out, same as the Windows EXE.
Copy-Item -Force 'proxy\gateway_proxy_parts\bundle_crypto.py' (Join-Path $partsDest 'bundle_crypto.py')

# Keep Resources/VERSION.txt aligned with the .app Info.plist (NOT root VERSION.txt).
# Root VERSION.txt may be ahead after a Windows-only rebuild; Mac auto-update must not loop.
$plist = 'release\Gateway_Guard.app\Contents\Info.plist'
$macVer = $null
if (Test-Path $plist) {
    try {
        $raw = Get-Content -Raw $plist
        if ($raw -match 'CFBundleShortVersionString</key>\s*<string>([^<]+)</string>') {
            $macVer = $Matches[1].Trim()
        }
    } catch {
        $macVer = $null
    }
}
if (-not $macVer -and (Test-Path 'release\VERSION.txt')) {
    $macVer = (Get-Content 'release\VERSION.txt' -Raw).Trim()
}
if ($macVer) {
    $resVer = Join-Path (Get-Location) 'release\Gateway_Guard.app\Contents\Resources\VERSION.txt'
    [System.IO.File]::WriteAllText($resVer, ($macVer.Trim() + "`n"))
    Write-Host "macOS .app Resources VERSION.txt = $macVer"
}

Get-ChildItem -Path 'release\Gateway_Guard.app' -Recurse -Directory -Filter '__pycache__' | Remove-Item -Recurse -Force -ErrorAction SilentlyContinue

$macStage = 'installer\staging-mac-zip'
if (Test-Path $macStage) { Remove-Item -Recurse -Force $macStage }
New-Item -ItemType Directory -Force -Path $macStage | Out-Null

Copy-Item -Recurse -Force 'release\Gateway_Guard.app' "$macStage\Gateway_Guard.app"
Copy-Item -Force 'release\gateway_guard_config.json' "$macStage\gateway_guard_config.json"
if ($macVer) {
    [System.IO.File]::WriteAllText((Join-Path (Get-Location) "$macStage\VERSION.txt"), ($macVer.Trim() + "`n"))
} elseif (Test-Path 'release\VERSION.txt') {
    Copy-Item -Force 'release\VERSION.txt' "$macStage\VERSION.txt"
}
if (Test-Path 'installer\EMPLOYEE_README_MAC.txt') { Copy-Item -Force 'installer\EMPLOYEE_README_MAC.txt' "$macStage\EMPLOYEE_README_MAC.txt" }
if (Test-Path 'release\INSTALL_MACOS.txt') { Copy-Item -Force 'release\INSTALL_MACOS.txt' "$macStage\INSTALL_MACOS.txt" }
if (Test-Path 'release\UNINSTALL_MACOS.txt') { Copy-Item -Force 'release\UNINSTALL_MACOS.txt' "$macStage\UNINSTALL_MACOS.txt" }
if (Test-Path 'release\Install_Gateway_Guard.command') { Copy-Item -Force 'release\Install_Gateway_Guard.command' "$macStage\Install_Gateway_Guard.command" }
if (Test-Path 'release\Uninstall_Gateway_Guard.command') { Copy-Item -Force 'release\Uninstall_Gateway_Guard.command' "$macStage\Uninstall_Gateway_Guard.command" }
if (Test-Path 'release\Update_Gateway_Guard_macOS.command') { Copy-Item -Force 'release\Update_Gateway_Guard_macOS.command' "$macStage\Update_Gateway_Guard_macOS.command" }

if (Test-Path 'release\Gateway_Guard_macOS.zip') { Remove-Item -Force 'release\Gateway_Guard_macOS.zip' }
Compress-Archive -Path "$macStage\*" -DestinationPath 'release\Gateway_Guard_macOS.zip' -Force

Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $macStage

Write-Host "macOS Guard package successfully rebuilt!"