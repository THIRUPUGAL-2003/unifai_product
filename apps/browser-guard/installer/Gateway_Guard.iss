; Gateway Guard — Enterprise Windows Installer (Inno Setup 6)
; Build: "C:\Program Files (x86)\Inno Setup 6\ISCC.exe" installer\Gateway_Guard.iss

#define MyAppName "Gateway Guard"
#define MyAppVersion "1.1.17"
#define MyAppPublisher "Gateway"
; Synced from .env SERVER_DOMAIN by apps/browser-guard/scripts/sync_config_from_env.py
#define MyAppURL "https://unifai.yespanchi.com"
#define MyAppExeName "Gateway_Guard.exe"
#define MyAppId "{8F3C2A91-6B4E-4D2F-9A71-A1B2C3D4E5F6}"

[Setup]
AppId={{8F3C2A91-6B4E-4D2F-9A71-A1B2C3D4E5F6}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
AppPublisherURL={#MyAppURL}
AppSupportURL={#MyAppURL}
DefaultDirName={localappdata}\Programs\Gateway\Guard
DefaultGroupName=Gateway Guard
DisableProgramGroupPage=yes
UsePreviousAppDir=no
UsePreviousGroup=no
OutputDir=..\release
OutputBaseFilename=Gateway_Guard_Setup
SetupIconFile=gateway_guard.ico
Compression=lzma
SolidCompression=yes
WizardStyle=modern
PrivilegesRequired=lowest
ArchitecturesInstallIn64BitMode=x64compatible
Uninstallable=no
CreateUninstallRegKey=no
CloseApplications=force
RestartApplications=no

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "autostart"; Description: "Start Gateway Guard automatically at Windows login (recommended)"; Flags: checkedonce

[Files]
Source: "staging\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs restartreplace
; Browser AI → Setup → Build now ships a server-fresh config next to Setup.exe; it wins over the compiled-in one.
Source: "{src}\gateway_guard_config.json"; DestDir: "{app}"; Flags: external skipifsourcedoesntexist ignoreversion

[Icons]
Name: "{group}\Gateway Guard"; Filename: "{app}\{#MyAppExeName}"
Name: "{group}\Uninstall Gateway Guard"; Filename: "{app}\{#MyAppExeName}"; Parameters: "--uninstall-prompt"
Name: "{userdesktop}\Gateway Guard"; Filename: "{app}\{#MyAppExeName}"; Tasks: autostart

[Registry]
; Permanent autostart for current user (HKCU) — required so PAC/proxy apply to the logged-in user
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "Gateway_Guard"; ValueData: """{app}\{#MyAppExeName}"""; Tasks: autostart
; Windows Control Panel ("Programs and Features") & Windows Settings ("Installed Apps") registration
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Uninstall\{{8F3C2A91-6B4E-4D2F-9A71-A1B2C3D4E5F6}"; ValueType: string; ValueName: "DisplayName"; ValueData: "{#MyAppName}"
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Uninstall\{{8F3C2A91-6B4E-4D2F-9A71-A1B2C3D4E5F6}"; ValueType: string; ValueName: "DisplayVersion"; ValueData: "{#MyAppVersion}"
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Uninstall\{{8F3C2A91-6B4E-4D2F-9A71-A1B2C3D4E5F6}"; ValueType: string; ValueName: "Publisher"; ValueData: "{#MyAppPublisher}"
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Uninstall\{{8F3C2A91-6B4E-4D2F-9A71-A1B2C3D4E5F6}"; ValueType: string; ValueName: "DisplayIcon"; ValueData: "{app}\{#MyAppExeName}"
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Uninstall\{{8F3C2A91-6B4E-4D2F-9A71-A1B2C3D4E5F6}"; ValueType: string; ValueName: "InstallLocation"; ValueData: "{app}"
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Uninstall\{{8F3C2A91-6B4E-4D2F-9A71-A1B2C3D4E5F6}"; ValueType: string; ValueName: "UninstallString"; ValueData: """{app}\{#MyAppExeName}"" --uninstall-prompt"
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Uninstall\{{8F3C2A91-6B4E-4D2F-9A71-A1B2C3D4E5F6}"; ValueType: string; ValueName: "QuietUninstallString"; ValueData: """{app}\{#MyAppExeName}"" --uninstall"
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Uninstall\{{8F3C2A91-6B4E-4D2F-9A71-A1B2C3D4E5F6}"; ValueType: string; ValueName: "HelpLink"; ValueData: "{#MyAppURL}"
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Uninstall\{{8F3C2A91-6B4E-4D2F-9A71-A1B2C3D4E5F6}"; ValueType: string; ValueName: "URLInfoAbout"; ValueData: "{#MyAppURL}"
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Uninstall\{{8F3C2A91-6B4E-4D2F-9A71-A1B2C3D4E5F6}"; ValueType: dword; ValueName: "NoModify"; ValueData: 1
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Uninstall\{{8F3C2A91-6B4E-4D2F-9A71-A1B2C3D4E5F6}"; ValueType: dword; ValueName: "NoRepair"; ValueData: 1
[Run]
; Run immediately in silent / auto-update mode
Filename: "{app}\{#MyAppExeName}"; Flags: nowait skipifnotsilent
; Checkbox on finish page in interactive mode
Filename: "{app}\{#MyAppExeName}"; Description: "Start Gateway Guard now"; Flags: nowait postinstall skipifsilent

[UninstallRun]
Filename: "{sys}\taskkill.exe"; Parameters: "/F /T /IM {#MyAppExeName}"; Flags: runhidden; RunOnceId: "StopGuard"

[UninstallDelete]
Type: filesandordirs; Name: "{localappdata}\Gateway\Guard"
Type: files; Name: "{app}\gateway_guard.log"
Type: files; Name: "{app}\proxy.pac"

[Code]
function StopRunningGuard(): Boolean;
var
  ResultCode: Integer;
begin
  // Terminate any currently running Gateway Guard so files can be cleanly replaced without lock errors
  Exec(ExpandConstant('{sys}\taskkill.exe'), '/F /T /IM {#MyAppExeName}', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  Sleep(500);
  Result := True;
end;

const
  UninstallKey = 'Software\Microsoft\Windows\CurrentVersion\Uninstall\{#MyAppId}_is1';

function PreviousInstallValue(const Name: String): String;
begin
  Result := '';
  if not RegQueryStringValue(HKCU, UninstallKey, Name, Result) then
    RegQueryStringValue(HKLM, UninstallKey, Name, Result);
end;

// An earlier build under this AppId may live in another <Vendor>\Guard folder with its own EXE
// name, autostart value and data dir. Left running, both Guards fight over the PAC/proxy, so
// stop and remove it, keeping its data dir (device identity) for this build.
procedure RemovePreviousBuild();
var
  ResultCode: Integer;
  OldDir, OldGroup, Vendor, OldData, NewData, Script: String;
begin
  OldDir := RemoveBackslashUnlessRoot(PreviousInstallValue('InstallLocation'));
  if (OldDir = '') or SameText(OldDir, RemoveBackslashUnlessRoot(ExpandConstant('{app}'))) then
    exit;
  if not SameText(ExtractFileName(OldDir), 'Guard') then
    exit;

  // Also cancels the "ping ... & start <old exe>" relaunch an auto-updating old Guard queued.
  Script := ExpandConstant('{tmp}\stop_previous_guard.ps1');
  SaveStringToFile(Script,
    'param([string]$d)' + #13#10 +
    '$d = $d.TrimEnd(''\'') + ''\''' + #13#10 +
    '$c = [StringComparison]::OrdinalIgnoreCase' + #13#10 +
    'Get-CimInstance Win32_Process | Where-Object { $_.Name -eq ''cmd.exe'' -and $_.CommandLine -and $_.CommandLine.IndexOf($d, $c) -ge 0 } | Invoke-CimMethod -MethodName Terminate | Out-Null' + #13#10 +
    'Get-Process | Where-Object { $_.Path -and $_.Path.StartsWith($d, $c) } | Stop-Process -Force -ErrorAction SilentlyContinue' + #13#10 +
    '$k = ''HKCU:\Software\Microsoft\Windows\CurrentVersion\Run''' + #13#10 +
    '$p = Get-ItemProperty -Path $k -ErrorAction SilentlyContinue' + #13#10 +
    'if ($p) { foreach ($n in (Get-Item -Path $k).Property) { if (([string]$p.$n).IndexOf($d, $c) -ge 0) { Remove-ItemProperty -Path $k -Name $n -ErrorAction SilentlyContinue } } }' + #13#10 +
    '$s = New-Object -ComObject WScript.Shell' + #13#10 +
    'Get-ChildItem -Path ([Environment]::GetFolderPath(''Desktop'')) -Filter *.lnk -ErrorAction SilentlyContinue | Where-Object { ([string]$s.CreateShortcut($_.FullName).TargetPath).StartsWith($d, $c) } | Remove-Item -Force -ErrorAction SilentlyContinue' + #13#10,
    False);
  Exec(ExpandConstant('{sys}\WindowsPowerShell\v1.0\powershell.exe'),
    '-NoProfile -NonInteractive -ExecutionPolicy Bypass -File "' + Script + '" "' + OldDir + '"',
    '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  Sleep(500);

  OldGroup := PreviousInstallValue('Inno Setup: Icon Group');
  if (OldGroup <> '') and not SameText(OldGroup, 'Gateway Guard') then
    DelTree(ExpandConstant('{userprograms}\') + OldGroup, True, True, True);

  Vendor := ExtractFileName(ExtractFileDir(OldDir));
  if (Vendor <> '') and not SameText(Vendor, 'Gateway') then
  begin
    OldData := ExpandConstant('{localappdata}\') + Vendor + '\Guard';
    NewData := ExpandConstant('{localappdata}\Gateway\Guard');
    // proxy_bundle targets the old EXE and must not be reused.
    if DirExists(OldData) and (DirExists(NewData) or
      (Exec(ExpandConstant('{sys}\robocopy.exe'), '"' + OldData + '" "' + NewData + '" /E /XD proxy_bundle /R:1 /W:1 /NFL /NDL /NJH /NJS',
        '', SW_HIDE, ewWaitUntilTerminated, ResultCode) and (ResultCode < 8))) then
    begin
      DelTree(OldData, True, True, True);
      RemoveDir(ExtractFileDir(OldData));
    end;
  end;

  DelTree(OldDir, True, True, True);
  RemoveDir(ExtractFileDir(OldDir));
end;

function InitializeSetup(): Boolean;
begin
  StopRunningGuard();
  Result := True;
end;

function PrepareToInstall(var NeedsRestart: Boolean): String;
var
  AppDir: String;
begin
  StopRunningGuard();
  RemovePreviousBuild();
  AppDir := ExpandConstant('{app}');
  if DirExists(AppDir) then
  begin
    if (not FileExists(AppDir + '\unins000.exe')) and FileExists(AppDir + '\unins000.dat') then
    begin
      DeleteFile(AppDir + '\unins000.dat');
    end;
  end;
  Result := '';
end;

function InitializeUninstall(): Boolean;
var
  ResultCode: Integer;
  ExePath: String;
begin
  Result := True;
  ExePath := ExpandConstant('{app}\{#MyAppExeName}');
  if FileExists(ExePath) then
  begin
    // Same PyInstaller env reset as [Run]: a Guard-launched uninstaller passes its env down.
    if Exec(ExpandConstant('{cmd}'), '/c set "PYINSTALLER_RESET_ENVIRONMENT=1" && "' + ExePath + '" --uninstall-prompt', ExpandConstant('{app}'), SW_HIDE, ewWaitUntilTerminated, ResultCode) then
    begin
      if ResultCode <> 0 then
      begin
        if ResultCode <> 3 then
          MsgBox('Uninstall rejected. Check the company uninstall key in Browser AI → Setup.', mbError, MB_OK);
        Result := False;
      end;
    end
    else
    begin
      MsgBox('Could not run Guard uninstall check. Aborting.', mbError, MB_OK);
      Result := False;
    end;
  end;
end;

procedure ShowAntivirusWarning(Reason: String);
begin
  MsgBox('SECURITY NOTICE: Gateway Guard installation or startup was blocked.' + #13#10 + #13#10 +
         'Reason: ' + Reason + #13#10 + #13#10 +
         'Your Antivirus or Windows Security (e.g. Defender, CrowdStrike, McAfee, Kaspersky, Norton, Bitdefender) may be blocking this enterprise security app.' + #13#10 + #13#10 +
         'Steps to resolve:' + #13#10 +
         '1. Open Windows Security or your company Antivirus app.' + #13#10 +
         '2. Under "Protection history" or "Quarantined threats", find Gateway_Guard.exe and choose "Allow on device" or "Restore".' + #13#10 +
         '3. Add an Exclusion / Whitelist for the installation folder:' + #13#10 +
         '   ' + ExpandConstant('{app}') + #13#10 +
         '4. If using a company-managed laptop, contact your IT Administrator with this message so they can approve Gateway Guard in your central security console.' + #13#10 +
         '5. Re-run this installer once allowed.',
         mbCriticalError, MB_OK);
end;

procedure CurStepChanged(CurStep: TSetupStep);
var
  ExePath: String;
begin
  if CurStep = ssPostInstall then
  begin
    ExePath := ExpandConstant('{app}\{#MyAppExeName}');
    if not FileExists(ExePath) then
    begin
      ShowAntivirusWarning('The application executable "Gateway_Guard.exe" was blocked, quarantined, or removed by your Antivirus during file extraction.');
    end;
  end;
end;

