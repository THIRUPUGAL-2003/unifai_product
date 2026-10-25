# Completely uninstall Gateway Guard and clean up all extension hooks, proxies, and files
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
Write-Host "=========================================================="
Write-Host "     Gateway Guard & Extension Complete Local Removal     "
Write-Host "=========================================================="

# 1. Stop all Guard processes and background browser instances
Write-Host "`n[1/6] Stopping running Guard and browser processes..."
Get-Process -Name "Gateway_Guard", "Gateway_Guard_Setup" -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Get-CimInstance Win32_Process | Where-Object { $_.CommandLine -match "gateway_agent\.py|agent_pac_server\.py" } | ForEach-Object {
    try { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue } catch {}
}
# Close browsers so shortcuts can be cleanly modified and locks released
Get-Process -Name "chrome", "msedge", "brave" -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Start-Sleep -Milliseconds 800

# 2. Reset Windows Proxy and PAC URL to completely direct
Write-Host "`n[2/6] Restoring Windows Internet Settings (Removing PAC Proxy)..."
$inetKey = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings"
try {
    Set-ItemProperty -Path $inetKey -Name "ProxyEnable" -Value 0 -Type DWord -Force -ErrorAction SilentlyContinue
    Remove-ItemProperty -Path $inetKey -Name "AutoConfigURL" -Force -ErrorAction SilentlyContinue
    Remove-ItemProperty -Path $inetKey -Name "ProxyServer" -Force -ErrorAction SilentlyContinue
    Remove-ItemProperty -Path $inetKey -Name "ProxyOverride" -Force -ErrorAction SilentlyContinue
    Write-Host "  -> Internet Settings restored to Direct Connection (AutoConfigURL removed)."
} catch {
    Write-Host "  -> [WARN] Error updating proxy registry: $_"
}

# Flush system internet options cache
try {
    $signature = @'
[DllImport("wininet.dll", SetLastError = true, CharSet=CharSet.Auto)]
public static extern bool InternetSetOption(IntPtr hInternet, int dwOption, IntPtr lpBuffer, int dwBufferLength);
'@
    $wininet = Add-Type -MemberDefinition $signature -Name "WinINetProxyFlush" -Namespace "GuardClean" -PassThru -ErrorAction SilentlyContinue
    if ($wininet) {
        [GuardClean.WinINetProxyFlush]::InternetSetOption([IntPtr]::Zero, 39, [IntPtr]::Zero, 0) | Out-Null # INTERNET_OPTION_SETTINGS_CHANGED
        [GuardClean.WinINetProxyFlush]::InternetSetOption([IntPtr]::Zero, 37, [IntPtr]::Zero, 0) | Out-Null # INTERNET_OPTION_REFRESH
        Write-Host "  -> WinINet options refreshed successfully."
    }
} catch {}

# 3. Clean up browser shortcuts (Remove --load-extension from Chrome, Edge, Brave)
Write-Host "`n[3/6] Cleaning up browser shortcuts (Removing --load-extension)..."
$shApp = New-Object -ComObject Shell.Application
$wsh = New-Object -ComObject WScript.Shell

$searchDirs = @(
    [Environment]::GetFolderPath('Desktop'),
    [Environment]::GetFolderPath('CommonDesktopDirectory'),
    [Environment]::GetFolderPath('Programs'),
    [Environment]::GetFolderPath('CommonPrograms'),
    "$env:APPDATA\Microsoft\Internet Explorer\Quick Launch\User Pinned\TaskBar",
    "$env:APPDATA\Microsoft\Internet Explorer\Quick Launch"
)

$oneDrive = "$env:USERPROFILE\OneDrive"
if (Test-Path $oneDrive) {
    Get-ChildItem -Path $oneDrive -Filter "*Desktop*" -Directory -Depth 2 -ErrorAction SilentlyContinue | ForEach-Object {
        $searchDirs += $_.FullName
    }
}

foreach ($dir in $searchDirs) {
    if (Test-Path $dir) {
        # Shell.Application (Unicode support)
        try {
            $folder = $shApp.Namespace($dir)
            if ($folder) {
                foreach ($item in $folder.Items()) {
                    if ($item.Name -match "chrome|edge|brave") {
                        if ($item.GetLink) {
                            $link = $item.GetLink
                            $cur = $link.Arguments
                            if ($cur -match "--load-extension") {
                                $clean = ($cur -replace '--load-extension=[^\s"]+', '' -replace '--load-extension="[^"]*"', '').Trim()
                                $link.Arguments = $clean
                                $link.Save()
                                Write-Host "  -> Cleaned [Shell.App]: $($item.Name) -> $clean"
                            }
                        }
                    }
                }
            }
        } catch {}

        # WScript.Shell fallback
        Get-ChildItem -Path $dir -Filter *.lnk -Depth 2 -ErrorAction SilentlyContinue | ForEach-Object {
            $name = $_.Name.ToLower()
            if ($name -match 'chrome' -or $name -match 'edge' -or $name -match 'brave') {
                try {
                    $linkPath = $_.FullName
                    try {
                        $fso = New-Object -ComObject Scripting.FileSystemObject
                        $linkPath = $fso.GetFile($_.FullName).ShortPath
                    } catch {}
                    $sc = $wsh.CreateShortcut($linkPath)
                    if ($sc.Arguments -match "--load-extension") {
                        $clean = ($sc.Arguments -replace '--load-extension=[^\s"]+', '' -replace '--load-extension="[^"]*"', '').Trim()
                        $sc.Arguments = $clean
                        $sc.Save()
                        Write-Host "  -> Cleaned [WScript]: $($_.FullName) -> $clean"
                    }
                } catch {}
            }
        }
    }
}

# 4. Remove Guard Shortcuts from Desktop and Start Menu
Write-Host "`n[4/6] Removing Gateway Guard shortcuts..."
$guardShortcuts = @(
    (Join-Path ([Environment]::GetFolderPath('Desktop')) "Gateway Guard.lnk"),
    (Join-Path ([Environment]::GetFolderPath('Desktop')) "UnifAI Guard.lnk"),
    (Join-Path "$env:USERPROFILE\OneDrive\ドキュメント\Desktop" "Gateway Guard.lnk"),
    (Join-Path "$env:USERPROFILE\OneDrive\ドキュメント\Desktop" "UnifAI Guard.lnk"),
    "$env:APPDATA\Microsoft\Windows\Start Menu\Programs\Gateway Guard",
    "$env:PROGRAMDATA\Microsoft\Windows\Start Menu\Programs\Gateway Guard"
)
foreach ($gs in $guardShortcuts) {
    if (Test-Path $gs) {
        Remove-Item -Recurse -Force $gs -ErrorAction SilentlyContinue
        Write-Host "  -> Removed shortcut/folder: $gs"
    }
}

# 5. Clean up Registry policies and auto-start entries
Write-Host "`n[5/6] Cleaning up Windows Registry entries..."
$registryKeysToRemove = @(
    "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run\Gateway Guard",
    "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run\Gateway_Guard",
    "HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Run\Gateway Guard",
    "HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Run\Gateway_Guard",
    "HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\Gateway Guard_is1",
    "HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\Gateway Guard_is1",
    "HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\{8F3C2A91-6B4E-4D2F-9A71-A1B2C3D4E5F6}",
    "HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\{8F3C2A91-6B4E-4D2F-9A71-A1B2C3D4E5F6}",
    "HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\{8F3C2A91-6B4E-4D2F-9A71-A1B2C3D4E5F6}_is1",
    "HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\{8F3C2A91-6B4E-4D2F-9A71-A1B2C3D4E5F6}_is1",
    "HKLM:\SOFTWARE\Microsoft\Edge\Extensions\mipfhcmlcjfhmgldkokdckhndkebkllj",
    "HKCU:\SOFTWARE\Microsoft\Edge\Extensions\mipfhcmlcjfhmgldkokdckhndkebkllj",
    "HKLM:\SOFTWARE\Google\Chrome\Extensions\mipfhcmlcjfhmgldkokdckhndkebkllj",
    "HKCU:\SOFTWARE\Google\Chrome\Extensions\mipfhcmlcjfhmgldkokdckhndkebkllj",
    "HKLM:\SOFTWARE\BraveSoftware\Brave\Extensions\mipfhcmlcjfhmgldkokdckhndkebkllj",
    "HKCU:\SOFTWARE\BraveSoftware\Brave\Extensions\mipfhcmlcjfhmgldkokdckhndkebkllj",
    "HKLM:\SOFTWARE\Policies\Google\Chrome\ExtensionInstallForcelist",
    "HKCU:\SOFTWARE\Policies\Google\Chrome\ExtensionInstallForcelist",
    "HKLM:\SOFTWARE\Policies\Microsoft\Edge\ExtensionInstallForcelist",
    "HKCU:\SOFTWARE\Policies\Microsoft\Edge\ExtensionInstallForcelist",
    "HKLM:\SOFTWARE\Policies\BraveSoftware\Brave\ExtensionInstallForcelist",
    "HKCU:\SOFTWARE\Policies\BraveSoftware\Brave\ExtensionInstallForcelist",
    "HKLM:\SOFTWARE\Policies\Google\Chrome\ExtensionInstallSources",
    "HKCU:\SOFTWARE\Policies\Google\Chrome\ExtensionInstallSources",
    "HKLM:\SOFTWARE\Policies\Microsoft\Edge\ExtensionInstallSources",
    "HKCU:\SOFTWARE\Policies\Microsoft\Edge\ExtensionInstallSources",
    "HKLM:\SOFTWARE\Policies\Google\Chrome\ExtensionSettings",
    "HKCU:\SOFTWARE\Policies\Google\Chrome\ExtensionSettings",
    "HKLM:\SOFTWARE\Policies\Microsoft\Edge\ExtensionSettings",
    "HKCU:\SOFTWARE\Policies\Microsoft\Edge\ExtensionSettings"
)
foreach ($rk in $registryKeysToRemove) {
    if (Test-Path $rk) {
        Remove-Item -Path $rk -Recurse -Force -ErrorAction SilentlyContinue
        Write-Host "  -> Removed registry key: $rk"
    } else {
        # Check if it's a value under Run
        if ($rk -match "Run\\(.+)$") {
            $valName = $matches[1]
            $parent = $rk.Substring(0, $rk.LastIndexOf("\"))
            try {
                if (Get-ItemProperty -Path $parent -Name $valName -ErrorAction SilentlyContinue) {
                    Remove-ItemProperty -Path $parent -Name $valName -Force -ErrorAction SilentlyContinue
                    Write-Host "  -> Removed startup entry: $valName from $parent"
                }
            } catch {}
        }
    }
}

# Clean up Desktop AI & IDE environment variables
$desktopEnvVars = @("NODE_EXTRA_CA_CERTS", "REQUESTS_CA_BUNDLE", "SSL_CERT_FILE", "CURL_CA_BUNDLE")
foreach ($ev in $desktopEnvVars) {
    if (Get-ItemProperty -Path "HKCU:\Environment" -Name $ev -ErrorAction SilentlyContinue) {
        Remove-ItemProperty -Path "HKCU:\Environment" -Name $ev -Force -ErrorAction SilentlyContinue
        Write-Host "  -> Removed Desktop AI environment variable: $ev"
    }
}

# 6. Delete local Gateway Guard application and extension files
Write-Host "`n[6/6] Deleting installed Gateway Guard application files..."
$foldersToDelete = @(
    "$env:ProgramData\Gateway",
    "$env:LOCALAPPDATA\Gateway",
    "$env:APPDATA\Gateway",
    "C:\Program Files (x86)\Gateway\Guard",
    "C:\Program Files (x86)\Gateway",
    "C:\Program Files\Gateway\Guard",
    "C:\Program Files\Gateway"
)
foreach ($folder in $foldersToDelete) {
    if (Test-Path $folder) {
        try {
            Remove-Item -Recurse -Force $folder -ErrorAction SilentlyContinue
            Write-Host "  -> Deleted folder: $folder"
        } catch {
            Write-Host "  -> [WARN] Could not completely remove $($folder) - error: $($_.Exception.Message)"
        }
    }
}

Write-Host "`n=========================================================="
Write-Host "  [SUCCESS] All Gateway Guard and Extension components    "
Write-Host "  have been completely removed from your local laptop!    "
Write-Host "=========================================================="
