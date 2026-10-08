@echo off
setlocal EnableExtensions
title Build Gateway Guard Enterprise Installer 1.1

cd /d "%~dp0.."

echo ============================================================
echo  Preflight: config + sources
echo ============================================================
if not exist agent\gateway_agent.py (
  echo Missing agent\gateway_agent.py
  exit /b 1
)
if not exist proxy\browser_ai_proxy.py (
  echo Missing proxy\browser_ai_proxy.py
  exit /b 1
)
if not exist config\gateway_guard_config.json (
  echo Missing config\gateway_guard_config.json
  exit /b 1
)

findstr /C:"backend_url" config\gateway_guard_config.json >nul
if errorlevel 1 (
  echo WARNING: config\gateway_guard_config.json missing backend_url — run sync_config_from_env.py
)
python scripts\sync_config_from_env.py
if errorlevel 1 (
  echo ERROR: set SERVER_DOMAIN in repo .env then: python apps/browser-guard/scripts/sync_config_from_env.py
  exit /b 1
)

echo.
echo ============================================================
echo  1) Building Gateway_Guard.exe  (embeds latest browser_ai_proxy.py)
echo ============================================================
python installer\build_agent.py
if errorlevel 1 (
  echo EXE build failed.
  exit /b 1
)

echo.
echo ============================================================
echo  2) Preparing installer staging
echo ============================================================
if not exist installer\staging mkdir installer\staging
if not exist release mkdir release

if exist build\exe\Gateway_Guard.exe (
  copy /Y build\exe\Gateway_Guard.exe installer\staging\Gateway_Guard.exe >nul
) else (
  copy /Y dist\Gateway_Guard.exe installer\staging\Gateway_Guard.exe >nul
)
copy /Y release\gateway_guard_config.json installer\staging\gateway_guard_config.json >nul
copy /Y gateway_guard.ico installer\staging\gateway_guard.ico >nul
copy /Y gateway_guard.ico installer\gateway_guard.ico >nul
if exist installer\EMPLOYEE_README.txt copy /Y installer\EMPLOYEE_README.txt installer\staging\EMPLOYEE_README.txt >nul
if exist installer\Uninstall_Gateway_Guard.bat copy /Y installer\Uninstall_Gateway_Guard.bat installer\staging\Uninstall_Gateway_Guard.bat >nul
if exist release\INSTALL_WINDOWS.txt copy /Y release\INSTALL_WINDOWS.txt installer\staging\INSTALL_WINDOWS.txt >nul
if exist release\VERSION.txt copy /Y release\VERSION.txt installer\staging\VERSION.txt >nul

echo.
echo ============================================================
echo  3) Compiling Setup EXE (Inno Setup)
echo ============================================================
set ISCC="%LOCALAPPDATA%\Programs\Inno Setup 6\ISCC.exe"
if not exist %ISCC% set ISCC="C:\Program Files (x86)\Inno Setup 6\ISCC.exe"
if not exist %ISCC% set ISCC="C:\Program Files\Inno Setup 6\ISCC.exe"
if not exist %ISCC% set ISCC="%LocalAppData%\Programs\Inno Setup 6\ISCC.exe"
if not exist %ISCC% (
  echo Inno Setup 6 not found. Staging folder is ready at installer\staging
  echo Install Inno Setup, then re-run this script.
  echo Raw EXE: dist\Gateway_Guard.exe
  exit /b 1
)

%ISCC% installer\Gateway_Guard.iss
if errorlevel 1 (
  echo Inno compile failed.
  exit /b 1
)

echo.
echo ============================================================
echo  4) Creating Windows employee ZIP
echo ============================================================
if exist release\Gateway_Guard_Windows.zip del /F /Q release\Gateway_Guard_Windows.zip

rem -- Stage the Setup EXE + portable EXE into a temp zip folder
set ZIP_STAGE=installer\staging-win-zip
if exist "%ZIP_STAGE%" rmdir /S /Q "%ZIP_STAGE%"
mkdir "%ZIP_STAGE%"

copy /Y release\Gateway_Guard_Setup.exe "%ZIP_STAGE%\Gateway_Guard_Setup.exe" >nul
if not exist "%ZIP_STAGE%\portable" mkdir "%ZIP_STAGE%\portable"
if exist build\exe\Gateway_Guard.exe (
  copy /Y build\exe\Gateway_Guard.exe "%ZIP_STAGE%\portable\Gateway_Guard.exe" >nul
) else if exist release\Gateway_Guard.exe (
  copy /Y release\Gateway_Guard.exe "%ZIP_STAGE%\portable\Gateway_Guard.exe" >nul
) else if exist dist\Gateway_Guard.exe (
  copy /Y dist\Gateway_Guard.exe "%ZIP_STAGE%\portable\Gateway_Guard.exe" >nul
)
copy /Y release\gateway_guard_config.json "%ZIP_STAGE%\gateway_guard_config.json" >nul
if exist release\INSTALL_WINDOWS.txt copy /Y release\INSTALL_WINDOWS.txt "%ZIP_STAGE%\INSTALL_WINDOWS.txt" >nul
if exist installer\EMPLOYEE_README.txt copy /Y installer\EMPLOYEE_README.txt "%ZIP_STAGE%\EMPLOYEE_README.txt" >nul
if exist installer\Uninstall_Gateway_Guard.bat copy /Y installer\Uninstall_Gateway_Guard.bat "%ZIP_STAGE%\Uninstall_Gateway_Guard.bat" >nul
if exist release\Update_Gateway_Guard.ps1 copy /Y release\Update_Gateway_Guard.ps1 "%ZIP_STAGE%\Update_Gateway_Guard.ps1" >nul
if exist release\VERSION.txt copy /Y release\VERSION.txt "%ZIP_STAGE%\VERSION.txt" >nul

rem -- Use PowerShell Compress-Archive (Windows 10+) to build the ZIP
powershell -NoProfile -Command "Compress-Archive -Path '%ZIP_STAGE%\*' -DestinationPath 'release\Gateway_Guard_Windows.zip' -Force"
if errorlevel 1 (
  echo WARNING: PowerShell Compress-Archive failed — Windows ZIP not created.
  echo   Distribute release\Gateway_Guard_Setup.exe directly to employees.
) else (
  echo Windows ZIP: release\Gateway_Guard_Windows.zip
)
rmdir /S /Q "%ZIP_STAGE%"

echo.
echo ============================================================
echo  SUCCESS — Gateway Guard 1.1
echo  Employee installer (Setup EXE):
echo    release\Gateway_Guard_Setup.exe
echo  Employee download package (Windows ZIP):
echo    release\Gateway_Guard_Windows.zip
echo  Portable EXE:
echo    release\Gateway_Guard.exe  (and dist\Gateway_Guard.exe)
echo  Backend: (from .env SERVER_DOMAIN — see config\gateway_guard_config.json)
echo ============================================================
if exist release\Gateway_Guard_Windows.zip (
  dir release\Gateway_Guard_Windows.zip
)
dir release\Gateway_Guard_Setup.exe
if exist release\Gateway_Guard.exe dir release\Gateway_Guard.exe
endlocal
