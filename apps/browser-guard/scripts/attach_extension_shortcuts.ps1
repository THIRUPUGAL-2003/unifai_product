[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$extPath = "$env:ProgramData\Gateway\Guard\guard_extension"
if (!(Test-Path $extPath)) { $extPath = "$env:LOCALAPPDATA\Gateway\Guard\guard_extension" }
$argToAdd = "--load-extension=`"$extPath`""

$wsh = New-Object -ComObject WScript.Shell
$shApp = New-Object -ComObject Shell.Application

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
        try {
            $folder = $shApp.Namespace($dir)
            if ($folder) {
                foreach ($item in $folder.Items()) {
                    if ($item.Name -match "chrome|edge|brave") {
                        if ($item.GetLink) {
                            $link = $item.GetLink
                            $cur = $link.Arguments
                            $clean = ($cur -replace '--load-extension=[^\s"]+', '' -replace '--load-extension="[^"]*"', '').Trim()
                            $link.Arguments = ($clean + " " + $argToAdd).Trim()
                            $link.Save()
                            Write-Host "[OK Shell.App] $($item.Name) -> $($link.Arguments)"
                        }
                    }
                }
            }
        } catch {}

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
                    $target = $sc.TargetPath
                    if ($target -match 'chrome\.exe' -or $target -match 'msedge\.exe' -or $target -match 'brave\.exe') {
                        $currentArgs = $sc.Arguments
                        $clean = ($currentArgs -replace '--load-extension=[^\s"]+', '' -replace '--load-extension="[^"]*"', '').Trim()
                        $sc.Arguments = ($clean + " " + $argToAdd).Trim()
                        $sc.Save()
                        Write-Host "[OK WScript] $($_.FullName) -> $($sc.Arguments)"
                    }
                } catch {
                    Write-Host "[WARN] Could not update $($_.FullName): $_"
                }
            }
        }
    }
}

# Ensure Edge exists on user Desktop
$desk = [Environment]::GetFolderPath("Desktop")
$edgeDesk = Join-Path $desk "Microsoft Edge.lnk"
if (!(Test-Path $edgeDesk)) {
    $startEdge = "$env:APPDATA\Microsoft\Windows\Start Menu\Programs\Microsoft Edge.lnk"
    if (Test-Path $startEdge) {
        Copy-Item -Path $startEdge -Destination $edgeDesk -Force -ErrorAction SilentlyContinue
        Write-Host "[OK] Created Edge Desktop shortcut"
    }
}

Write-Host "Shortcut attachment completed."
