param (
    [string]$Key = "",
    [switch]$GenerateClusterSecret
)

# Feature: Generate a new cryptographically secure 48-byte cluster secret
if ($GenerateClusterSecret) {
    $bytes = New-Object byte[] 36
    $rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
    $rng.GetBytes($bytes)
    $newSecret = [Convert]::ToBase64String($bytes).Replace('+', '-').Replace('/', '_').TrimEnd('=')
    Write-Host "==========================================================" -ForegroundColor Green
    Write-Host " Generated New Cryptographic Cluster Secret (48 chars)" -ForegroundColor Green
    Write-Host "==========================================================" -ForegroundColor Green
    Write-Host "CLUSTER_REPLICATE_SECRET=$newSecret" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "Copy this line into the .env file of ALL cluster nodes (Node 1, Node 2, Node 3)." -ForegroundColor Yellow
    exit 0
}

$envFile = Join-Path $PSScriptRoot "..\.env"

if (-not (Test-Path $envFile)) {
    Write-Host "[ERROR] .env file not found at: $envFile" -ForegroundColor Red
    exit 1
}

$lines = Get-Content $envFile | Where-Object { $_ -match "^[A-Za-z0-9_]+=" }

$envMap = @{}
foreach ($line in $lines) {
    $parts = $line.Split("=", 2)
    if ($parts.Length -eq 2) {
        $envMap[$parts[0].Trim()] = $parts[1].Trim()
    }
}

if ($Key -ne "") {
    if ($envMap.ContainsKey($Key)) {
        $val = $envMap[$Key]
        Write-Host "$Key = $val" -ForegroundColor Cyan
    } else {
        Write-Host "[NOT FOUND] Key '$Key' does not exist in .env" -ForegroundColor Yellow
    }
    exit 0
}

Write-Host "==========================================================" -ForegroundColor Green
Write-Host "   UnifAI Active Configuration (.env) Overview" -ForegroundColor Green
Write-Host "==========================================================" -ForegroundColor Green

foreach ($k in ($envMap.Keys | Sort-Object)) {
    $v = $envMap[$k]
    if ($k -match "PASSWORD|SECRET|KEY" -and $k -ne "CLUSTER_REPLICATE_SECRET") {
        $v = "<redacted>"
    }
    Write-Host ("{0,-30} = {1}" -f $k, $v)
}

Write-Host "==========================================================" -ForegroundColor Green
