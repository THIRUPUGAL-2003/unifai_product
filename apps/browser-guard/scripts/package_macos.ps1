Set-Location -Path $PSScriptRoot\..

# Sync fleet config + live proxy scripts into the shipped .app (Windows host can do this;
# full Darwin binary rebuild still requires make build-guard-mac on a Mac).
$appPath = 'release\YP_Raksha.app'
if (-not (Test-Path $appPath)) {
    if (Test-Path 'release\Gateway_Guard.app') {
        Copy-Item -Recurse -Force 'release\Gateway_Guard.app' $appPath
    } else {
        Write-Error "$appPath missing - copy a Mac-built .app first (make build-guard-mac)."
        exit 1
    }
}

Copy-Item -Force 'release\gateway_guard_config.json' "$appPath\Contents\Resources\gateway_guard_config.json"
Copy-Item -Force 'installer\proxy_stub.py' "$appPath\Contents\Resources\browser_ai_proxy.py"
if (-not (Test-Path 'proxy\gateway_guard_code.enc')) {
    Write-Error "proxy\gateway_guard_code.enc missing. Run installer\encrypt_proxy_bundle.py before packaging."
    exit 1
}
Copy-Item -Force 'proxy\gateway_guard_code.enc' "$appPath\Contents\Resources\gateway_guard_code.enc"
$partsDest = "$appPath\Contents\Resources\gateway_proxy_parts"
New-Item -ItemType Directory -Force -Path $partsDest | Out-Null
if (-not (Test-Path 'proxy\gateway_proxy_parts\gateway_proxy_parts.enc')) {
    Write-Error "proxy\gateway_proxy_parts\gateway_proxy_parts.enc missing. Run installer\encrypt_proxy_bundle.py before packaging."
    exit 1
}
Get-ChildItem $partsDest -File -ErrorAction SilentlyContinue | Where-Object { $_.Extension -eq '.py' -or $_.Name -eq 'README.md' -or $_.Name -eq 'Gateway_proxy_parts.enc' } | Remove-Item -Force -ErrorAction SilentlyContinue
Copy-Item -Force 'proxy\gateway_proxy_parts\gateway_proxy_parts.enc' (Join-Path $partsDest 'gateway_proxy_parts.enc')
if (Test-Path 'proxy\gateway_proxy_parts\gateway_proxy_parts\MANIFEST.txt') {
    Copy-Item -Force 'proxy\gateway_proxy_parts\gateway_proxy_parts\MANIFEST.txt' (Join-Path $partsDest 'MANIFEST.txt')
} elseif (Test-Path 'proxy\gateway_proxy_parts\MANIFEST.txt') {
    Copy-Item -Force 'proxy\gateway_proxy_parts\MANIFEST.txt' (Join-Path $partsDest 'MANIFEST.txt')
}
# Decryptor is compiled into the Mac binary (hidden import). Do not ship bundle_crypto.py.
Get-ChildItem -Path $appPath -Recurse -File -Filter '*.py' |
    Where-Object { $_.Name -ne 'browser_ai_proxy.py' } |
    Remove-Item -Force -ErrorAction SilentlyContinue

# Keep Resources/VERSION.txt aligned with the .app Info.plist (NOT root VERSION.txt).
$plist = "$appPath\Contents\Info.plist"
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
    $resVer = Join-Path (Get-Location) "$appPath\Contents\Resources\VERSION.txt"
    [System.IO.File]::WriteAllText($resVer, ($macVer.Trim() + "`n"))
    Write-Host "macOS .app Resources VERSION.txt = $macVer"
}

Get-ChildItem -Path $appPath -Recurse -Directory -Filter '__pycache__' | Remove-Item -Recurse -Force -ErrorAction SilentlyContinue

$macStage = 'installer\staging-mac-zip'
if (Test-Path $macStage) { Remove-Item -Recurse -Force $macStage }
New-Item -ItemType Directory -Force -Path $macStage | Out-Null

Copy-Item -Recurse -Force $appPath "$macStage\YP_Raksha.app"
Copy-Item -Force 'release\gateway_guard_config.json' "$macStage\gateway_guard_config.json"
if ($macVer) {
    [System.IO.File]::WriteAllText((Join-Path (Get-Location) "$macStage\VERSION.txt"), ($macVer.Trim() + "`n"))
} elseif (Test-Path 'release\VERSION.txt') {
    Copy-Item -Force 'release\VERSION.txt' "$macStage\VERSION.txt"
}
if (Test-Path 'release\EMPLOYEE_README_MAC.txt') { Copy-Item -Force 'release\EMPLOYEE_README_MAC.txt' "$macStage\EMPLOYEE_README_MAC.txt" }
if (Test-Path 'release\INSTALL_MACOS.txt') { Copy-Item -Force 'release\INSTALL_MACOS.txt' "$macStage\INSTALL_MACOS.txt" }
if (Test-Path 'release\UNINSTALL_MACOS.txt') { Copy-Item -Force 'release\UNINSTALL_MACOS.txt' "$macStage\UNINSTALL_MACOS.txt" }
if (Test-Path 'release\Install_YP_Raksha.command') { Copy-Item -Force 'release\Install_YP_Raksha.command' "$macStage\Install_YP_Raksha.command" }
if (Test-Path 'release\Uninstall_YP_Raksha.command') { Copy-Item -Force 'release\Uninstall_YP_Raksha.command' "$macStage\Uninstall_YP_Raksha.command" }
if (Test-Path 'release\Update_YP_Raksha_macOS.command') { Copy-Item -Force 'release\Update_YP_Raksha_macOS.command' "$macStage\Update_YP_Raksha_macOS.command" }

if (Test-Path 'release\Gateway_Guard_macOS.zip') { Remove-Item -Force 'release\Gateway_Guard_macOS.zip' }
if (Test-Path 'release\YP_Raksha_macOS.zip') { Remove-Item -Force 'release\YP_Raksha_macOS.zip' }
python -c "import shutil; shutil.make_archive('release/YP_Raksha_macOS', 'zip', 'installer/staging-mac-zip')"

Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $macStage

Write-Host "macOS YP Raksha package successfully rebuilt (release\YP_Raksha_macOS.zip)!"