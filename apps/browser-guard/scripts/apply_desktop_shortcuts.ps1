$wsh = New-Object -ComObject WScript.Shell
$extPath = "C:\Users\sakth\AppData\Local\Gateway\Guard\guard_extension"
$desktop = "C:\Users\sakth\OneDrive\9831~1\Desktop"

$lnkChrome = "$desktop\Thirupugal (Your Chrome) - Chrome.lnk"
if (Test-Path $lnkChrome) {
    $scChrome = $wsh.CreateShortcut($lnkChrome)
    Write-Host "Chrome Target: $($scChrome.TargetPath)"
    Write-Host "Chrome Current Args: $($scChrome.Arguments)"
    $cur = $scChrome.Arguments
    if ($cur -notmatch 'load-extension') {
        $scChrome.Arguments = ($cur + " --load-extension=`"$extPath`"").Trim()
        $scChrome.Save()
        Write-Host "[SUCCESS] Chrome Desktop Shortcut updated!"
    } else {
        Write-Host "[ALREADY] Chrome Desktop Shortcut has load-extension"
    }
}

$lnkBrave = "$desktop\Brave.lnk"
if (Test-Path $lnkBrave) {
    $scBrave = $wsh.CreateShortcut($lnkBrave)
    Write-Host "Brave Target: $($scBrave.TargetPath)"
    Write-Host "Brave Current Args: $($scBrave.Arguments)"
    $curB = $scBrave.Arguments
    if ($curB -notmatch 'load-extension') {
        $scBrave.Arguments = ($curB + " --load-extension=`"$extPath`"").Trim()
        $scBrave.Save()
        Write-Host "[SUCCESS] Brave Desktop Shortcut updated!"
    } else {
        Write-Host "[ALREADY] Brave Desktop Shortcut has load-extension"
    }
}
