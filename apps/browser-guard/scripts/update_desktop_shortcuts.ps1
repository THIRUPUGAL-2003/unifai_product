$wsh = New-Object -ComObject WScript.Shell
$extPath = "$env:ProgramData\Gateway\Guard\guard_extension"
if (!(Test-Path $extPath)) { $extPath = "$env:LOCALAPPDATA\Gateway\Guard\guard_extension" }
$argToAdd = "--load-extension=`"$extPath`""

$searchDirs = @(
    [Environment]::GetFolderPath("Desktop"),
    "$env:USERPROFILE\Desktop",
    "$env:USERPROFILE\OneDrive\Desktop"
)
$files = @()
foreach ($d in $searchDirs) {
    if (Test-Path $d) {
        $files += Get-ChildItem -Path $d -Filter "*.lnk" -ErrorAction SilentlyContinue | Where-Object { $_.Name -match 'Chrome' -or $_.Name -match 'Brave' -or $_.Name -match 'Edge' }
    }
}
foreach ($f in $files) {
    try {
        $sc = $wsh.CreateShortcut($f.FullName)
        $cur = $sc.Arguments
        if ($cur -notmatch 'load-extension') {
            $sc.Arguments = ($cur + " " + $argToAdd).Trim()
            $sc.Save()
            Write-Host "[OK] Updated $($f.Name)"
        } else {
            Write-Host "[ALREADY] $($f.Name)"
        }
    } catch {
        Write-Host "[WARN] $($f.Name) : $_"
    }
}
