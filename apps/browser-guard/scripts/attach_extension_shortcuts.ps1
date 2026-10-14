$extPath = "$env:LOCALAPPDATA\Gateway\Guard\guard_extension"
$argToAdd = "--load-extension=`"$extPath`""

$wsh = New-Object -ComObject WScript.Shell
$desktop = [Environment]::GetFolderPath('Desktop')
$publicDesktop = [Environment]::GetFolderPath('CommonDesktopDirectory')
$startMenu = [Environment]::GetFolderPath('Programs')
$commonStartMenu = [Environment]::GetFolderPath('CommonPrograms')
$taskbar = "$env:APPDATA\Microsoft\Internet Explorer\Quick Launch\User Pinned\TaskBar"

$searchDirs = @($desktop, $publicDesktop, $startMenu, $commonStartMenu, $taskbar)

foreach ($dir in $searchDirs) {
    if (Test-Path $dir) {
        Get-ChildItem -Path $dir -Filter *.lnk -Recurse -ErrorAction SilentlyContinue | ForEach-Object {
            $name = $_.Name.ToLower()
            if ($name -match 'chrome' -or $name -match 'edge' -or $name -match 'brave') {
                try {
                    $sc = $wsh.CreateShortcut($_.FullName)
                    $target = $sc.TargetPath
                    if ($target -match 'chrome\.exe' -or $target -match 'msedge\.exe' -or $target -match 'brave\.exe') {
                        $currentArgs = $sc.Arguments
                        if ($currentArgs -notmatch 'load-extension') {
                            $sc.Arguments = ($currentArgs + " " + $argToAdd).Trim()
                            $sc.Save()
                            Write-Host "[OK] Updated $($_.FullName) with --load-extension"
                        } else {
                            Write-Host "[ALREADY] $($_.FullName) already has --load-extension"
                        }
                    }
                } catch {
                    Write-Host "[WARN] Could not update $($_.FullName): $_"
                }
            }
        }
    }
}
Write-Host "Shortcut update done."
