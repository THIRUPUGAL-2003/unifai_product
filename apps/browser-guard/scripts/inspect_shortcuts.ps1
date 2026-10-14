$sh = New-Object -ComObject WScript.Shell
$desktop = [Environment]::GetFolderPath('Desktop')
Get-ChildItem -Path $desktop -Filter *.lnk | ForEach-Object {
    $sc = $sh.CreateShortcut($_.FullName)
    Write-Host "$($_.Name) -> $($sc.TargetPath) | $($sc.Arguments)"
}

$taskbar = "$env:APPDATA\Microsoft\Internet Explorer\Quick Launch\User Pinned\TaskBar"
if (Test-Path $taskbar) {
    Get-ChildItem -Path $taskbar -Filter *.lnk | ForEach-Object {
        $sc = $sh.CreateShortcut($_.FullName)
        Write-Host "Taskbar: $($_.Name) -> $($sc.TargetPath) | $($sc.Arguments)"
    }
}
