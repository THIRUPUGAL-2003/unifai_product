$exe = "$env:LOCALAPPDATA\Programs\Gateway\Guard\Gateway_Guard.exe"
if (!(Test-Path $exe)) {
    Write-Host "EXE does not exist at $exe"
    exit 1
}

$p = Start-Process -FilePath $exe -PassThru
Start-Sleep -Seconds 3
if ($p.HasExited) {
    Write-Host "Process exited with code: $($p.ExitCode)"
    $log = "$env:LOCALAPPDATA\Gateway\Guard\gateway_guard.log"
    if (Test-Path $log) {
        Write-Host "--- LAST 30 LINES OF LOG ---"
        Get-Content $log -Tail 30
    }
} else {
    Write-Host "Process is successfully running with PID: $($p.Id)"
    $net = Get-NetTCPConnection -LocalPort 18195 -State Listen -ErrorAction SilentlyContinue
    if ($net) {
        Write-Host "Port 18195 is ACTIVE and LISTENING!"
    } else {
        Write-Host "Port 18195 is not listening yet."
    }
}
