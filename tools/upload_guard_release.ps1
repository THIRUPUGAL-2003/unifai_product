<#
.SYNOPSIS
  Upload Raksha Guard installers to the server (they are not stored in git).

.EXAMPLE
  .\tools\upload_guard_release.ps1 -Server root@203.0.113.10 -RemoteDir /opt/raksha/apps/browser-guard/release

  RemoteDir = the server folder mounted into the container as /app/release
  (docker-compose.yml: ./apps/browser-guard/release:/app/release). No restart needed -
  the dashboard serves the new files immediately; press Rebuild & Publish afterwards.
#>
param(
    [Parameter(Mandatory = $false)][string]$Server = "root@76.13.243.253",
    [Parameter(Mandatory = $false)][string]$RemoteDir = "/opt/projects/unifai_product/apps/browser-guard/release",
    [int]$Port = 22,
    [string]$IdentityFile = ""
)

$ErrorActionPreference = "Stop"
$releaseDir = Join-Path $PSScriptRoot "..\apps\browser-guard\release"
# Only untracked binaries: uploading git-tracked files would make `git pull` on the server fail.
$names = @("Raksha_Guard_Setup.exe", "Raksha_Guard.exe", "Raksha_Guard_Windows.zip", "Raksha_Guard_macOS.zip", "Raksha_Guard_Setup.pkg")

foreach ($tool in "ssh", "scp") {
    if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) {
        throw "$tool not found - enable Windows 'OpenSSH Client' (Settings > Optional features) or upload via the 1Panel file manager."
    }
}

$sshArgs = @("-p", "$Port")
$scpArgs = @("-P", "$Port")
if ($IdentityFile) {
    $sshArgs += @("-i", $IdentityFile)
    $scpArgs += @("-i", $IdentityFile)
}

$files = @()
foreach ($n in $names) {
    $p = Join-Path $releaseDir $n
    if (Test-Path $p) { $files += Get-Item $p } else { Write-Host "skip (not built): $n" }
}
if ($files.Count -eq 0) { throw "No Guard binaries in $releaseDir - build them first." }

$remote = $RemoteDir.TrimEnd("/")
& ssh @sshArgs $Server "mkdir -p '$remote'"
if ($LASTEXITCODE -ne 0) { throw "ssh to $Server failed." }

foreach ($f in $files) {
    $mb = [math]::Round($f.Length / 1MB, 1)
    Write-Host "Uploading $($f.Name) ($mb MB)..."
    # Upload under a temp name, then rename, so downloads never see a half-written installer.
    & scp @scpArgs $f.FullName "${Server}:$remote/.$($f.Name).part"
    if ($LASTEXITCODE -ne 0) { throw "scp failed for $($f.Name)." }
    & ssh @sshArgs $Server "mv -f '$remote/.$($f.Name).part' '$remote/$($f.Name)'"
    if ($LASTEXITCODE -ne 0) { throw "rename failed for $($f.Name)." }
}

Write-Host "Syncing into running Docker container (unifai_version)..."
& ssh @sshArgs $Server "docker cp '$remote/.' unifai_version:/app/release/ 2>/dev/null; docker cp '$remote/.' unifai_version:/app/apps/browser-guard/release/ 2>/dev/null; ls -la '$remote'"
Write-Host ""
Write-Host "Done! Open Browser AI > Setup (https://unifai.yespanchi.com), confirm Windows and macOS show Ready, then click 'Rebuild & Publish'."
