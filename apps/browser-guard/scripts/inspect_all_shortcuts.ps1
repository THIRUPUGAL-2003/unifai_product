$sh = New-Object -ComObject WScript.Shell
$desk = [Environment]::GetFolderPath('Desktop')
Write-Host "Desktop path: $desk"
Get-ChildItem -Path $desk -Filter *.lnk | ForEach-Object {
    $sc = $sh.CreateShortcut($_.FullName)
    Write-Host "DESKTOP SHORTCUT: $($_.Name)"
    Write-Host "  TargetPath: $($sc.TargetPath)"
    Write-Host "  Arguments:  $($sc.Arguments)"
}

Write-Host "=== CHECKING RUNNING PROCESS COMMAND LINES ==="
Get-CimInstance Win32_Process | Where-Object { $_.Name -match 'chrome\.exe|msedge\.exe|brave\.exe' } | Select-Object -First 6 | ForEach-Object {
    Write-Host "$($_.Name) [PID $($_.ProcessId)]:"
    Write-Host "   $($_.CommandLine)"
}

Write-Host "=== CHECKING REGISTRY POLICIES ==="
Get-ItemProperty -Path 'HKLM:\SOFTWARE\Policies\Google\Chrome\ExtensionInstallForcelist' -ErrorAction SilentlyContinue
Get-ItemProperty -Path 'HKLM:\SOFTWARE\Policies\Microsoft\Edge\ExtensionInstallForcelist' -ErrorAction SilentlyContinue
Get-ItemProperty -Path 'HKLM:\SOFTWARE\Policies\BraveSoftware\Brave\ExtensionInstallForcelist' -ErrorAction SilentlyContinue
Get-ItemProperty -Path 'HKCU:\SOFTWARE\Policies\Google\Chrome\ExtensionInstallForcelist' -ErrorAction SilentlyContinue
Get-ItemProperty -Path 'HKCU:\SOFTWARE\Policies\Microsoft\Edge\ExtensionInstallForcelist' -ErrorAction SilentlyContinue
