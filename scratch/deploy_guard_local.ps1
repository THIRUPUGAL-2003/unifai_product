Write-Host "Stopping running Gateway Guard..."
Get-Process -Name 'Gateway_Guard' -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep -Seconds 2

Write-Host "Running silent install of freshly built Gateway_Guard_Setup.exe..."
$setup = "d:\unifai_project\apps\browser-guard\release\Gateway_Guard_Setup.exe"
Start-Process -FilePath $setup -ArgumentList "/VERYSILENT /NORESTART /SUPPRESSMSGBOXES" -Wait
Start-Sleep -Seconds 3

Write-Host "Launching updated Gateway Guard..."
$guardExe = Join-Path $env:LOCALAPPDATA "Programs\Gateway\Guard\Gateway_Guard.exe"
Start-Process -FilePath $guardExe
Start-Sleep -Seconds 4

Write-Host "Checking running Gateway Guard processes:"
Get-Process -Name 'Gateway_Guard' -ErrorAction SilentlyContinue | Select-Object Id, ProcessName, Path
