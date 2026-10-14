$wsh = New-Object -ComObject WScript.Shell
$extPath = "$env:LOCALAPPDATA\Gateway\Guard\guard_extension"
$argToAdd = "--load-extension=`"$extPath`""

$files = Get-ChildItem -Path "C:\Users\sakth\OneDrive" -Filter "*.lnk" -Recurse -ErrorAction SilentlyContinue | Where-Object { $_.Name -match 'Chrome' -or $_.Name -match 'Brave' -or $_.Name -match 'Edge' }
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
