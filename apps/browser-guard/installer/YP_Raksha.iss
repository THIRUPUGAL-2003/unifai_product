#define MyAppName "YP Raksha"
#define MyAppVersion "1.1.33"
#define MyAppPublisher "YesPanchi"

[Setup]
AppId={{8F3C2A91-6B4E-4D2F-9A71-A1B2C3D4E5F6}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
DefaultDirName={localappdata}\Programs\YP Raksha
DefaultGroupName={#MyAppName}
DisableDirPage=yes
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
ArchitecturesAllowed=x64
ArchitecturesInstallIn64BitMode=x64
Uninstallable=no
CloseApplications=yes
RestartApplications=no
OutputDir=output
OutputBaseFilename=YP_Raksha_Setup
SetupIconFile=yp_raksha.ico
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
VersionInfoVersion=1.1.33.0
VersionInfoCompany={#MyAppPublisher}
VersionInfoDescription=YP Raksha Setup
VersionInfoProductName={#MyAppName}
VersionInfoProductVersion={#MyAppVersion}

[Files]
Source: "staging\YP_Raksha.exe"; DestDir: "{app}"; Flags: ignoreversion restartreplace
Source: "staging\gateway_guard_config.json"; DestDir: "{app}"; Flags: ignoreversion
Source: "staging\VERSION.txt"; DestDir: "{app}"; Flags: ignoreversion skipifsourcedoesntexist
Source: "staging\yp_raksha.ico"; DestDir: "{app}"; Flags: ignoreversion
Source: "staging\Uninstall_YP_Raksha.bat"; DestDir: "{app}"; Flags: ignoreversion
Source: "staging\EMPLOYEE_README.txt"; DestDir: "{app}"; Flags: ignoreversion
Source: "staging\INSTALL_WINDOWS.txt"; DestDir: "{app}"; Flags: ignoreversion skipifsourcedoesntexist

[Icons]
Name: "{group}\YP Raksha"; Filename: "{app}\YP_Raksha.exe"; WorkingDir: "{app}"; IconFilename: "{app}\yp_raksha.ico"
Name: "{group}\Uninstall YP Raksha"; Filename: "{app}\YP_Raksha.exe"; Parameters: "--uninstall-prompt"; WorkingDir: "{app}"; IconFilename: "{app}\yp_raksha.ico"

[Registry]
; The agent owns authorized uninstall. Do not create an Inno uninstaller that bypasses its key check.
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Uninstall\{{8F3C2A91-6B4E-4D2F-9A71-A1B2C3D4E5F6}"; ValueType: string; ValueName: "DisplayName"; ValueData: "{#MyAppName}"
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Uninstall\{{8F3C2A91-6B4E-4D2F-9A71-A1B2C3D4E5F6}"; ValueType: string; ValueName: "DisplayVersion"; ValueData: "{#MyAppVersion}"
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Uninstall\{{8F3C2A91-6B4E-4D2F-9A71-A1B2C3D4E5F6}"; ValueType: string; ValueName: "Publisher"; ValueData: "{#MyAppPublisher}"
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Uninstall\{{8F3C2A91-6B4E-4D2F-9A71-A1B2C3D4E5F6}"; ValueType: string; ValueName: "DisplayIcon"; ValueData: "{app}\YP_Raksha.exe"
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Uninstall\{{8F3C2A91-6B4E-4D2F-9A71-A1B2C3D4E5F6}"; ValueType: string; ValueName: "InstallLocation"; ValueData: "{app}"
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Uninstall\{{8F3C2A91-6B4E-4D2F-9A71-A1B2C3D4E5F6}"; ValueType: string; ValueName: "UninstallString"; ValueData: """{app}\YP_Raksha.exe"" --uninstall-prompt"
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Uninstall\{{8F3C2A91-6B4E-4D2F-9A71-A1B2C3D4E5F6}"; ValueType: dword; ValueName: "NoModify"; ValueData: "1"
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Uninstall\{{8F3C2A91-6B4E-4D2F-9A71-A1B2C3D4E5F6}"; ValueType: dword; ValueName: "NoRepair"; ValueData: "1"

[Run]
Filename: "{app}\YP_Raksha.exe"; Description: "Launch YP Raksha"; WorkingDir: "{app}"; Flags: postinstall nowait

[Code]
function PrepareToInstall(var NeedsRestart: Boolean): String;
var
  ResultCode: Integer;
begin
  { Upgrade / reinstall over a running Guard: stop it (and its MitM worker), then drop the
    deny-delete rule it puts on its own folder, or replacing YP_Raksha.exe fails. }
  Exec(ExpandConstant('{sys}\taskkill.exe'), '/F /T /IM YP_Raksha.exe', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  Sleep(800);
  if DirExists(ExpandConstant('{app}')) then
    Exec(ExpandConstant('{sys}\icacls.exe'), '"' + ExpandConstant('{app}') + '" /remove:d "' + GetUserNameString + '" /T /C /Q', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  Result := '';
end;
