[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$desk = [Environment]::GetFolderPath('Desktop')
Write-Host "Desk: $desk"
$chromeLnk = Get-ChildItem -Path $desk -Filter "*Chrome*.lnk"
foreach ($f in $chromeLnk) {
    Write-Host "Found file: $($f.FullName)"
    $sh = New-Object -ComObject WScript.Shell
    $sc = $sh.CreateShortcut($f.FullName)
    Write-Host "WScript Target: '$($sc.TargetPath)'"
    Write-Host "WScript Args:   '$($sc.Arguments)'"

    $shellApp = New-Object -ComObject Shell.Application
    $dir = Split-Path $f.FullName
    $leaf = Split-Path $f.FullName -Leaf
    $ns = $shellApp.Namespace($dir)
    $item = $ns.ParseName($leaf)
    if ($item -and $item.GetLink) {
        Write-Host "Shell.App Target: '$($item.GetLink.Target.Path)'"
        Write-Host "Shell.App Args:   '$($item.GetLink.Arguments)'"
    }
}
