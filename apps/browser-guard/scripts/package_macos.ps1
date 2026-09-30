Set-Location -Path $PSScriptRoot\..

# Sync fleet config + live proxy scripts into the shipped .app (Windows host can do this;
# full Darwin binary rebuild still requires make build-guard-mac on a Mac).
if (-not (Test-Path 'release\UnifAI_Guard.app')) {
    Write-Error "release\UnifAI_Guard.app missing - copy a Mac-built .app first (make build-guard-mac)."
    exit 1
}

Copy-Item -Force 'release\unifai_guard_config.json' 'release\UnifAI_Guard.app\Contents\Resources\unifai_guard_config.json'
Copy-Item -Force 'proxy\browser_ai_proxy.py' 'release\UnifAI_Guard.app\Contents\Resources\browser_ai_proxy.py'
Copy-Item -Recurse -Force 'proxy\unifai_proxy_parts\*' 'release\UnifAI_Guard.app\Contents\Resources\unifai_proxy_parts\'

# Keep Resources/VERSION.txt aligned with the .app Info.plist (NOT root VERSION.txt).
# Root VERSION.txt may be ahead after a Windows-only rebuild; Mac auto-update must not loop.
$plist = 'release\UnifAI_Guard.app\Contents\Info.plist'
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
    $resVer = Join-Path (Get-Location) 'release\UnifAI_Guard.app\Contents\Resources\VERSION.txt'
    [System.IO.File]::WriteAllText($resVer, ($macVer.Trim() + "`n"))
    Write-Host "macOS .app Resources VERSION.txt = $macVer"
}

Get-ChildItem -Path 'release\UnifAI_Guard.app' -Recurse -Directory -Filter '__pycache__' | Remove-Item -Recurse -Force -ErrorAction SilentlyContinue

$macStage = 'installer\staging-mac-zip'
if (Test-Path $macStage) { Remove-Item -Recurse -Force $macStage }
New-Item -ItemType Directory -Force -Path $macStage | Out-Null

Copy-Item -Recurse -Force 'release\UnifAI_Guard.app' "$macStage\UnifAI_Guard.app"
Copy-Item -Force 'release\unifai_guard_config.json' "$macStage\unifai_guard_config.json"
if ($macVer) {
    [System.IO.File]::WriteAllText((Join-Path (Get-Location) "$macStage\VERSION.txt"), ($macVer.Trim() + "`n"))
} elseif (Test-Path 'release\VERSION.txt') {
    Copy-Item -Force 'release\VERSION.txt' "$macStage\VERSION.txt"
}
if (Test-Path 'installer\EMPLOYEE_README_MAC.txt') { Copy-Item -Force 'installer\EMPLOYEE_README_MAC.txt' "$macStage\EMPLOYEE_README_MAC.txt" }
if (Test-Path 'release\INSTALL_MACOS.txt') { Copy-Item -Force 'release\INSTALL_MACOS.txt' "$macStage\INSTALL_MACOS.txt" }
if (Test-Path 'release\UNINSTALL_MACOS.txt') { Copy-Item -Force 'release\UNINSTALL_MACOS.txt' "$macStage\UNINSTALL_MACOS.txt" }
if (Test-Path 'release\Install_UnifAI_Guard.command') { Copy-Item -Force 'release\Install_UnifAI_Guard.command' "$macStage\Install_UnifAI_Guard.command" }
if (Test-Path 'release\Uninstall_UnifAI_Guard.command') { Copy-Item -Force 'release\Uninstall_UnifAI_Guard.command' "$macStage\Uninstall_UnifAI_Guard.command" }
if (Test-Path 'release\Update_UnifAI_Guard_macOS.command') { Copy-Item -Force 'release\Update_UnifAI_Guard_macOS.command' "$macStage\Update_UnifAI_Guard_macOS.command" }

if (Test-Path 'release\UnifAI_Guard_macOS.zip') { Remove-Item -Force 'release\UnifAI_Guard_macOS.zip' }
Compress-Archive -Path "$macStage\*" -DestinationPath 'release\UnifAI_Guard_macOS.zip' -Force

Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $macStage

Write-Host "macOS Guard package successfully rebuilt!"