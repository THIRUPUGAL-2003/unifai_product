Set-Location -Path $PSScriptRoot\..

# 0. Sync configs from root .env
if (Test-Path 'scripts\sync_config_from_env.py') {
    Write-Host "Syncing Guard config from root .env..."
    python scripts\sync_config_from_env.py
}

# 1. Prepare Staging
New-Item -ItemType Directory -Force -Path 'installer\staging' | Out-Null
$guardExe = $null
if (Test-Path 'dist\Gateway_Guard.exe') {
    $guardExe = 'dist\Gateway_Guard.exe'
} elseif (Test-Path 'release\Gateway_Guard.exe') {
    $guardExe = 'release\Gateway_Guard.exe'
}
if (-not $guardExe) {
    Write-Error "Gateway_Guard.exe missing - run installer\build_agent.py (or build_installer.bat) first."
    exit 1
}
Copy-Item -Force $guardExe 'installer\staging\Gateway_Guard.exe'
Copy-Item -Force 'release\gateway_guard_config.json' 'installer\staging\gateway_guard_config.json'
Copy-Item -Force 'gateway_guard.ico' 'installer\staging\gateway_guard.ico'
if (Test-Path 'installer\EMPLOYEE_README.txt') { Copy-Item -Force 'installer\EMPLOYEE_README.txt' 'installer\staging\EMPLOYEE_README.txt' }
if (Test-Path 'installer\Uninstall_Gateway_Guard.bat') { Copy-Item -Force 'installer\Uninstall_Gateway_Guard.bat' 'installer\staging\Uninstall_Gateway_Guard.bat' }
if (Test-Path 'release\INSTALL_WINDOWS.txt') { Copy-Item -Force 'release\INSTALL_WINDOWS.txt' 'installer\staging\INSTALL_WINDOWS.txt' }
if (Test-Path 'release\VERSION.txt') { Copy-Item -Force 'release\VERSION.txt' 'installer\staging\VERSION.txt' }

# 2. Compile Inno Setup (same search order as build_installer.bat - no machine-specific paths)
$isccCandidates = @(
    (Join-Path $env:LOCALAPPDATA 'Programs\Inno Setup 6\ISCC.exe'),
    'C:\Program Files (x86)\Inno Setup 6\ISCC.exe',
    'C:\Program Files\Inno Setup 6\ISCC.exe'
)
$iscc = $isccCandidates | Where-Object { Test-Path $_ } | Select-Object -First 1
if (-not $iscc) {
    Write-Error "Inno Setup 6 not found. Install it, then re-run. Staging is ready at installer\staging"
    exit 1
}
Write-Host "Running Inno Setup compiler: $iscc ..."
& $iscc 'installer\Gateway_Guard.iss'
if ($LASTEXITCODE -ne 0) { throw "Inno Setup compilation failed" }

# 3. Create Windows ZIP
$zipStage = 'installer\staging-win-zip'
if (Test-Path $zipStage) { Remove-Item -Recurse -Force $zipStage }
New-Item -ItemType Directory -Force -Path $zipStage | Out-Null

Copy-Item -Force 'release\Gateway_Guard_Setup.exe' "$zipStage\Gateway_Guard_Setup.exe"
if ($guardExe -and (Test-Path $guardExe)) {
    New-Item -ItemType Directory -Force -Path "$zipStage\portable" | Out-Null
    Copy-Item -Force $guardExe "$zipStage\portable\Gateway_Guard.exe"
}
Copy-Item -Force 'release\gateway_guard_config.json' "$zipStage\gateway_guard_config.json"
if (Test-Path 'release\INSTALL_WINDOWS.txt') { Copy-Item -Force 'release\INSTALL_WINDOWS.txt' "$zipStage\INSTALL_WINDOWS.txt" }
if (Test-Path 'installer\EMPLOYEE_README.txt') { Copy-Item -Force 'installer\EMPLOYEE_README.txt' "$zipStage\EMPLOYEE_README.txt" }
if (Test-Path 'release\Update_Gateway_Guard.ps1') { Copy-Item -Force 'release\Update_Gateway_Guard.ps1' "$zipStage\Update_Gateway_Guard.ps1" }
if (Test-Path 'release\Uninstall_Gateway_Guard.bat') { Copy-Item -Force 'release\Uninstall_Gateway_Guard.bat' "$zipStage\Uninstall_Gateway_Guard.bat" }
if (Test-Path 'release\VERSION.txt') { Copy-Item -Force 'release\VERSION.txt' "$zipStage\VERSION.txt" }

if (Test-Path 'release\Gateway_Guard_Windows.zip') { Remove-Item -Force 'release\Gateway_Guard_Windows.zip' }
Compress-Archive -Path "$zipStage\*" -DestinationPath 'release\Gateway_Guard_Windows.zip' -Force

# 4. Clean temporary staging and build folders
Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $zipStage
Remove-Item -Recurse -Force -ErrorAction SilentlyContinue 'installer\staging'
Remove-Item -Recurse -Force -ErrorAction SilentlyContinue 'build'
Remove-Item -Recurse -Force -ErrorAction SilentlyContinue 'dist'

Write-Host "Windows Guard build & package successfully completed!"
