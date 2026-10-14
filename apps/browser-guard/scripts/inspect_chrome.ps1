Get-CimInstance Win32_Process | Where-Object { $_.Name -eq 'chrome.exe' } | ForEach-Object {
    Write-Host "PID: $($_.ProcessId)"
    Write-Host "  CommandLine: $($_.CommandLine)"
}
