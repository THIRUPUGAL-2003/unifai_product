[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

$exe = "$env:LOCALAPPDATA\Programs\Gateway\Guard\Gateway_Guard.exe"
if (-not (Test-Path $exe)) {
    Write-Host "ERROR: $exe not found!"
    exit 1
}

$proc = Get-Process -Name "Gateway_Guard" -ErrorAction SilentlyContinue
if (-not $proc) {
    Write-Host "Starting $exe..."
    Start-Process -FilePath $exe
    Start-Sleep -Seconds 4
} else {
    Write-Host "Gateway_Guard is already running (PID: $($proc.Id -join ', '))"
}

$proc = Get-Process -Name "Gateway_Guard" -ErrorAction SilentlyContinue
if ($proc) {
    Write-Host "Process running: PID $($proc.Id -join ', ')"
} else {
    Write-Host "Process failed to stay running."
}

try {
    $resp = Invoke-WebRequest -Uri "http://127.0.0.1:18195/health" -TimeoutSec 3 -UseBasicParsing
    Write-Host "HEALTH CHECK: $($resp.Content)"
} catch {
    Write-Host "HEALTH CHECK FAILED: $($_.Exception.Message)"
}

try {
    $rules = Invoke-WebRequest -Uri "http://127.0.0.1:18195/extension/rules" -TimeoutSec 3 -UseBasicParsing
    Write-Host "RULES CHECK: Length=$($rules.Content.Length)"
} catch {
    Write-Host "RULES CHECK FAILED: $($_.Exception.Message)"
}

$proxy = Get-ItemProperty -Path "HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings"
Write-Host "SYSTEM PROXY: ProxyEnable=$($proxy.ProxyEnable), AutoConfigURL=$($proxy.AutoConfigURL)"
