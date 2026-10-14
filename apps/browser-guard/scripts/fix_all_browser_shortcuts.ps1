[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

$extPath = "C:\Users\sakth\AppData\Local\Gateway\Guard\guard_extension"
$argToAdd = "--load-extension=`"$extPath`""

$wsh = New-Object -ComObject WScript.Shell
$shellApp = New-Object -ComObject Shell.Application

# 1. Update or create Desktop shortcuts
$desktops = @(
    "C:\Users\sakth\Desktop",
    "C:\Users\sakth\OneDrive\ドキュメント\Desktop",
    "C:\Users\Public\Desktop"
)

# Also ensure Edge desktop shortcut exists
foreach ($d in $desktops) {
    if (Test-Path $d) {
        $edgeLnk = Join-Path $d "Microsoft Edge.lnk"
        if (!(Test-Path $edgeLnk)) {
            $edgeExe = "C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe"
            if (Test-Path $edgeExe) {
                try {
                    $sc = $wsh.CreateShortcut($edgeLnk)
                    $sc.TargetPath = $edgeExe
                    $sc.Arguments = $argToAdd
                    $sc.IconLocation = "$edgeExe,0"
                    $sc.Save()
                    Write-Host "[CREATED] Created Edge shortcut on $d"
                } catch {
                    Write-Host "[WARN] Could not create Edge lnk on $d : $_"
                }
            }
        }
    }
}

# 2. Update all existing lnk files across Desktop, Taskbar, Start Menu
$searchDirs = @(
    "C:\Users\sakth\Desktop",
    "C:\Users\sakth\OneDrive\ドキュメント\Desktop",
    "C:\Users\Public\Desktop",
    "$env:APPDATA\Microsoft\Windows\Start Menu\Programs",
    "$env:ProgramData\Microsoft\Windows\Start Menu\Programs",
    "$env:APPDATA\Microsoft\Internet Explorer\Quick Launch\User Pinned\TaskBar",
    "$env:APPDATA\Microsoft\Internet Explorer\Quick Launch"
)

foreach ($dir in $searchDirs) {
    if (Test-Path $dir) {
        Get-ChildItem -Path $dir -Filter *.lnk -Recurse -Depth 2 -ErrorAction SilentlyContinue | Where-Object { $_.Name -match 'chrome|edge|brave' } | ForEach-Object {
            try {
                $sc = $wsh.CreateShortcut($_.FullName)
                $target = $sc.TargetPath
                if ($target -match 'chrome\.exe' -or $target -match 'msedge\.exe' -or $target -match 'brave\.exe') {
                    $cur = $sc.Arguments
                    # Clean out any old/double-escaped load-extension flags
                    $cleaned = ($cur -replace '--load-extension=[^\s"]+', '' -replace '--load-extension="[^"]*"', '').Trim()
                    $sc.Arguments = ($cleaned + " " + $argToAdd).Trim()
                    $sc.Save()
                    Write-Host "[FIXED] $($_.FullName) -> $($sc.Arguments)"
                }
            } catch {
                Write-Host "[SKIP] $($_.FullName): $_"
            }
        }
    }
}

Write-Host "All shortcuts updated cleanly."
