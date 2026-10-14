$procId = (Get-NetTCPConnection -LocalPort 18195 -State Listen -ErrorAction SilentlyContinue).OwningProcess
if ($procId) {
    Get-Process -Id $procId | Select-Object Id, ProcessName, Path
} else {
    Write-Host "Nothing listening on 18195"
}
