[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$sh = New-Object -ComObject WScript.Shell
$shellApp = New-Object -ComObject Shell.Application

$paths = @(
    [Environment]::GetFolderPath("Desktop"),
    [Environment]::GetFolderPath("CommonDesktopDirectory"),
    "$env:USERPROFILE\Desktop",
    "$env:USERPROFILE\OneDrive\Desktop",
    "$env:PUBLIC\Desktop",
    "$env:APPDATA\Microsoft\Windows\Start Menu\Programs",
    "$env:ProgramData\Microsoft\Windows\Start Menu\Programs",
    "$env:APPDATA\Microsoft\Internet Explorer\Quick Launch\User Pinned\TaskBar",
    "$env:APPDATA\Microsoft\Internet Explorer\Quick Launch"
)

foreach ($dir in $paths) {
    if (Test-Path $dir) {
        Get-ChildItem -Path $dir -Filter *.lnk -Recurse -Depth 2 -ErrorAction SilentlyContinue | Where-Object { $_.Name -match 'chrome|edge|brave' } | ForEach-Object {
            $dirPath = Split-Path $_.FullName
            $leaf = Split-Path $_.FullName -Leaf
            $folder = $shellApp.Namespace($dirPath)
            $item = $folder.ParseName($leaf)
            $link = $item.GetLink
            Write-Host "LNK: $($_.FullName)"
            Write-Host "  Target: $($link.Target.Path)"
            Write-Host "  Args:   $($link.Arguments)"
        }
    }
}
