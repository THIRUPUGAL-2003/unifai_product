[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$shApp = New-Object -ComObject Shell.Application
$desk = [Environment]::GetFolderPath('Desktop')
Write-Host "Updating desktop shortcuts in: $desk"
$folder = $shApp.Namespace($desk)

$extPath = "C:\Users\sakth\AppData\Local\Gateway\Guard\guard_extension"
$argToAdd = "--load-extension=`"$extPath`""

# Check all items in desktop
foreach ($item in $folder.Items()) {
    if ($item.Name -match "Chrome" -or $item.Name -match "Edge" -or $item.Name -match "Brave") {
        if ($item.GetLink) {
            $link = $item.GetLink
            $tgt = $link.Target.Path
            $cur = $link.Arguments
            $clean = ($cur -replace '--load-extension=[^\s"]+', '' -replace '--load-extension="[^"]*"', '').Trim()
            $link.Arguments = ($clean + " " + $argToAdd).Trim()
            $link.Save()
            Write-Host "Updated $($item.Name): Target='$tgt' Args='$($link.Arguments)'"
        }
    }
}

# Also ensure Edge exists on Desktop
$edgeItem = $folder.ParseName("Microsoft Edge.lnk")
if (-not $edgeItem) {
    $edgeExe = "C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe"
    if (Test-Path $edgeExe) {
        $wsh = New-Object -ComObject WScript.Shell
        # Use short path or direct copy to bypass COM Unicode bug
        $edgeDeskPath = Join-Path $desk "Microsoft Edge.lnk"
        $startEdgeLnk = "$env:APPDATA\Microsoft\Windows\Start Menu\Programs\Microsoft Edge.lnk"
        if (Test-Path $startEdgeLnk) {
            Copy-Item -Path $startEdgeLnk -Destination $edgeDeskPath -Force
            Write-Host "Copied Microsoft Edge shortcut to Desktop from Start Menu!"
        }
    }
}
