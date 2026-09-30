@echo off
setlocal EnableExtensions
title Build UnifAI Guard Enterprise Installer 1.1

cd /d "%~dp0.."

echo ============================================================
echo  Preflight: config + sources
echo ============================================================
if not exist agent\unifai_agent.py (
  echo Missing agent\unifai_agent.py
  exit /b 1
)
if not exist proxy\browser_ai_proxy.py (
  echo Missing proxy\browser_ai_proxy.py
  exit /b 1
)
if not exist config\unifai_guard_config.json (
  echo Missing config\unifai_guard_config.json
  exit /b 1
)

findstr /C:"backend_url" config\unifai_guard_config.json >nul
if errorlevel 1 (
  echo WARNING: config\unifai_guard_config.json missing backend_url — run sync_config_from_env.py
)
python scripts\sync_config_from_env.py
if errorlevel 1 (
  echo ERROR: set SERVER_DOMAIN in repo .env then: python apps/browser-guard/scripts/sync_config_from_env.py
  exit /b 1
)

echo.
echo ============================================================
echo  1) Building UnifAI_Guard.exe  (embeds latest browser_ai_proxy.py)
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

copy /Y dist\UnifAI_Guard.exe installer\staging\UnifAI_Guard.exe >nul
copy /Y release\unifai_guard_config.json installer\staging\unifai_guard_config.json >nul
copy /Y unifai_guard.ico installer\staging\unifai_guard.ico >nul
copy /Y unifai_guard.ico installer\unifai_guard.ico >nul
if exist installer\EMPLOYEE_README.txt copy /Y installer\EMPLOYEE_README.txt installer\staging\EMPLOYEE_README.txt >nul
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
  echo Raw EXE: dist\UnifAI_Guard.exe
  exit /b 1
)

%ISCC% installer\UnifAI_Guard.iss
if errorlevel 1 (
  echo Inno compile failed.
  exit /b 1
)

echo.
echo ============================================================
echo  4) Creating Windows employee ZIP
echo ============================================================
if exist release\UnifAI_Guard_Windows.zip del /F /Q release\UnifAI_Guard_Windows.zip

rem -- Stage the Setup EXE + portable EXE into a temp zip folder
set ZIP_STAGE=installer\staging-win-zip
if exist "%ZIP_STAGE%" rmdir /S /Q "%ZIP_STAGE%"
mkdir "%ZIP_STAGE%"

copy /Y release\UnifAI_Guard_Setup.exe "%ZIP_STAGE%\UnifAI_Guard_Setup.exe" >nul
if exist release\UnifAI_Guard.exe copy /Y release\UnifAI_Guard.exe "%ZIP_STAGE%\UnifAI_Guard.exe" >nul
if exist dist\UnifAI_Guard.exe if not exist release\UnifAI_Guard.exe copy /Y dist\UnifAI_Guard.exe "%ZIP_STAGE%\UnifAI_Guard.exe" >nul
copy /Y release\unifai_guard_config.json "%ZIP_STAGE%\unifai_guard_config.json" >nul
if exist release\INSTALL_WINDOWS.txt copy /Y release\INSTALL_WINDOWS.txt "%ZIP_STAGE%\INSTALL_WINDOWS.txt" >nul
if exist installer\EMPLOYEE_README.txt copy /Y installer\EMPLOYEE_README.txt "%ZIP_STAGE%\EMPLOYEE_README.txt" >nul
if exist release\Update_UnifAI_Guard.ps1 copy /Y release\Update_UnifAI_Guard.ps1 "%ZIP_STAGE%\Update_UnifAI_Guard.ps1" >nul
if exist release\VERSION.txt copy /Y release\VERSION.txt "%ZIP_STAGE%\VERSION.txt" >nul

rem -- Use PowerShell Compress-Archive (Windows 10+) to build the ZIP
powershell -NoProfile -Command "Compress-Archive -Path '%ZIP_STAGE%\*' -DestinationPath 'release\UnifAI_Guard_Windows.zip' -Force"
if errorlevel 1 (
  echo WARNING: PowerShell Compress-Archive failed — Windows ZIP not created.
  echo   Distribute release\UnifAI_Guard_Setup.exe directly to employees.
) else (
  echo Windows ZIP: release\UnifAI_Guard_Windows.zip
)
rmdir /S /Q "%ZIP_STAGE%"

echo.
echo ============================================================
echo  SUCCESS — UnifAI Guard 1.1
echo  Employee installer (Setup EXE):
echo    release\UnifAI_Guard_Setup.exe
echo  Employee download package (Windows ZIP):
echo    release\UnifAI_Guard_Windows.zip
echo  Portable EXE:
echo    release\UnifAI_Guard.exe  (and dist\UnifAI_Guard.exe)
echo  Backend: (from .env SERVER_DOMAIN — see config\unifai_guard_config.json)
echo ============================================================
if exist release\UnifAI_Guard_Windows.zip (
  dir release\UnifAI_Guard_Windows.zip
)
dir release\UnifAI_Guard_Setup.exe
if exist release\UnifAI_Guard.exe dir release\UnifAI_Guard.exe
endlocal
