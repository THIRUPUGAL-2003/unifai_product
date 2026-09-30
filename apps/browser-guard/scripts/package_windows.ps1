Set-Location -Path $PSScriptRoot\..

# 1. Prepare Staging
New-Item -ItemType Directory -Force -Path 'installer\staging' | Out-Null
if (-not (Test-Path 'dist\UnifAI_Guard.exe')) {
    Write-Error "dist\UnifAI_Guard.exe missing — run installer\build_agent.py (or build_installer.bat) first."
    exit 1
}
Copy-Item -Force 'dist\UnifAI_Guard.exe' 'installer\staging\UnifAI_Guard.exe'
Copy-Item -Force 'release\unifai_guard_config.json' 'installer\staging\unifai_guard_config.json'
Copy-Item -Force 'unifai_guard.ico' 'installer\staging\unifai_guard.ico'
if (Test-Path 'installer\EMPLOYEE_README.txt') { Copy-Item -Force 'installer\EMPLOYEE_README.txt' 'installer\staging\EMPLOYEE_README.txt' }
if (Test-Path 'release\INSTALL_WINDOWS.txt') { Copy-Item -Force 'release\INSTALL_WINDOWS.txt' 'installer\staging\INSTALL_WINDOWS.txt' }
if (Test-Path 'release\VERSION.txt') { Copy-Item -Force 'release\VERSION.txt' 'installer\staging\VERSION.txt' }

# 2. Compile Inno Setup (same search order as build_installer.bat — no machine-specific paths)
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
& $iscc 'installer\UnifAI_Guard.iss'
if ($LASTEXITCODE -ne 0) { throw "Inno Setup compilation failed" }

# 3. Create Windows ZIP
$zipStage = 'installer\staging-win-zip'
if (Test-Path $zipStage) { Remove-Item -Recurse -Force $zipStage }
New-Item -ItemType Directory -Force -Path $zipStage | Out-Null

Copy-Item -Force 'release\UnifAI_Guard_Setup.exe' "$zipStage\UnifAI_Guard_Setup.exe"
Copy-Item -Force 'dist\UnifAI_Guard.exe' "$zipStage\UnifAI_Guard.exe"
Copy-Item -Force 'release\unifai_guard_config.json' "$zipStage\unifai_guard_config.json"
if (Test-Path 'release\INSTALL_WINDOWS.txt') { Copy-Item -Force 'release\INSTALL_WINDOWS.txt' "$zipStage\INSTALL_WINDOWS.txt" }
if (Test-Path 'installer\EMPLOYEE_README.txt') { Copy-Item -Force 'installer\EMPLOYEE_README.txt' "$zipStage\EMPLOYEE_README.txt" }
if (Test-Path 'release\Update_UnifAI_Guard.ps1') { Copy-Item -Force 'release\Update_UnifAI_Guard.ps1' "$zipStage\Update_UnifAI_Guard.ps1" }
if (Test-Path 'release\VERSION.txt') { Copy-Item -Force 'release\VERSION.txt' "$zipStage\VERSION.txt" }

if (Test-Path 'release\UnifAI_Guard_Windows.zip') { Remove-Item -Force 'release\UnifAI_Guard_Windows.zip' }
Compress-Archive -Path "$zipStage\*" -DestinationPath 'release\UnifAI_Guard_Windows.zip' -Force

# 4. Clean temporary staging and build folders
Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $zipStage
Remove-Item -Recurse -Force -ErrorAction SilentlyContinue 'installer\staging'
Remove-Item -Recurse -Force -ErrorAction SilentlyContinue 'build'
Remove-Item -Recurse -Force -ErrorAction SilentlyContinue 'dist'

Write-Host "Windows Guard build & package successfully completed!"
