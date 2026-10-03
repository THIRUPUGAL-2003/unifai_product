; Raksha Guard — Enterprise Windows Installer (Inno Setup 6)
; Build: "C:\Program Files (x86)\Inno Setup 6\ISCC.exe" installer\Raksha_Guard.iss

#define MyAppName "Raksha Guard"
#define MyAppVersion "1.1.15"
#define MyAppPublisher "Raksha"
; Synced from .env SERVER_DOMAIN by apps/browser-guard/scripts/sync_config_from_env.py
#define MyAppURL "https://unifai.yespanchi.com"
#define MyAppExeName "Raksha_Guard.exe"

[Setup]
AppId={{8F3C2A91-6B4E-4D2F-9A71-A1B2C3D4E5F6}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
AppPublisherURL={#MyAppURL}
AppSupportURL={#MyAppURL}
DefaultDirName={localappdata}\Programs\Raksha\Guard
DefaultGroupName=Raksha Guard
DisableProgramGroupPage=yes
OutputDir=..\release
OutputBaseFilename=Raksha_Guard_Setup
SetupIconFile=raksha_guard.ico
Compression=lzma
SolidCompression=yes
WizardStyle=modern
PrivilegesRequired=lowest
ArchitecturesInstallIn64BitMode=x64compatible
UninstallDisplayName={#MyAppName}
CloseApplications=force
RestartApplications=no

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "autostart"; Description: "Start Raksha Guard automatically at Windows login (recommended)"; Flags: checkedonce

[Files]
Source: "staging\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{group}\Raksha Guard"; Filename: "{app}\{#MyAppExeName}"
Name: "{group}\Uninstall Raksha Guard"; Filename: "{uninstallexe}"
Name: "{userdesktop}\Raksha Guard"; Filename: "{app}\{#MyAppExeName}"; Tasks: autostart

[Registry]
; Permanent autostart for current user (HKCU) — required so PAC/proxy apply to the logged-in user
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "Raksha_Guard"; ValueData: """{app}\{#MyAppExeName}"""; Flags: uninsdeletevalue; Tasks: autostart

[Run]
; No skipifsilent — auto-update uses /VERYSILENT and must restart Guard after replace.
; Via cmd with PYINSTALLER_RESET_ENVIRONMENT: an auto-updating Guard (onefile EXE) launches this
; installer, and its inherited PyInstaller env would make the new Guard fail
; ("Security validation failure: parent process has different executable").
Filename: "{cmd}"; Parameters: "/c set ""PYINSTALLER_RESET_ENVIRONMENT=1"" && start """" ""{app}\{#MyAppExeName}"""; Description: "Start Raksha Guard now"; Flags: nowait postinstall runhidden

[UninstallRun]
Filename: "{sys}\taskkill.exe"; Parameters: "/F /T /IM {#MyAppExeName}"; Flags: runhidden; RunOnceId: "StopGuard"

[UninstallDelete]
Type: filesandordirs; Name: "{localappdata}\Raksha\Guard"
Type: files; Name: "{app}\raksha_guard.log"
Type: files; Name: "{app}\proxy.pac"

[Code]
function StopRunningGuard(): Boolean;
var
  ResultCode: Integer;
begin
  // Terminate any currently running Raksha Guard so files can be cleanly replaced without lock errors
  Exec(ExpandConstant('{sys}\taskkill.exe'), '/F /T /IM {#MyAppExeName}', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  Sleep(500);
  Result := True;
end;

function InitializeSetup(): Boolean;
begin
  StopRunningGuard();
  Result := True;
end;

function PrepareToInstall(var NeedsRestart: Boolean): String;
begin
  StopRunningGuard();
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
