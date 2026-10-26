@echo off
setlocal EnableExtensions
title Build Gateway Guard Enterprise Installer 1.1

cd /d "%~dp0.."

set "ISCC=%LOCALAPPDATA%\Programs\Inno Setup 6\ISCC.exe"
if not exist "%ISCC%" set "ISCC=%USERPROFILE%\AppData\Local\Programs\Inno Setup 6\ISCC.exe"
if not exist "%ISCC%" set "ISCC=C:\Program Files (x86)\Inno Setup 6\ISCC.exe"
if not exist "%ISCC%" set "ISCC=C:\Program Files\Inno Setup 6\ISCC.exe"
if not exist "%ISCC%" (
  echo ERROR: Inno Setup 6 compiler not found. Install Inno Setup 6, then rerun this script.
  exit /b 1
)
if not exist installer\YP_Raksha.iss (
  echo ERROR: installer\YP_Raksha.iss is missing.
  exit /b 1
)

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
python scripts\pack_all_extensions.py
if errorlevel 1 (
  echo ERROR: browser extension packaging failed.
  exit /b 1
)
python installer\encrypt_proxy_bundle.py
if errorlevel 1 (
  echo ERROR: proxy bundle encryption failed.
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

if not exist build\exe\YP_Raksha.exe (
  echo ERROR: fresh build\exe\YP_Raksha.exe was not produced.
  exit /b 1
)
copy /Y build\exe\YP_Raksha.exe installer\staging\YP_Raksha.exe >nul
if errorlevel 1 (
  echo ERROR: could not stage the freshly built YP_Raksha.exe.
  exit /b 1
)
if exist build\exe\Gateway_Guard.exe (
  copy /Y build\exe\Gateway_Guard.exe installer\staging\Gateway_Guard.exe >nul
) else if exist dist\Gateway_Guard.exe (
  copy /Y dist\Gateway_Guard.exe installer\staging\Gateway_Guard.exe >nul
)
copy /Y release\gateway_guard_config.json installer\staging\gateway_guard_config.json >nul
if exist yp_raksha.ico (
  copy /Y yp_raksha.ico installer\staging\yp_raksha.ico >nul
  copy /Y yp_raksha.ico installer\yp_raksha.ico >nul
)
if exist gateway_guard.ico (
  copy /Y gateway_guard.ico installer\staging\gateway_guard.ico >nul
  copy /Y gateway_guard.ico installer\gateway_guard.ico >nul
)
if exist installer\EMPLOYEE_README.txt copy /Y installer\EMPLOYEE_README.txt installer\staging\EMPLOYEE_README.txt >nul
copy /Y installer\Uninstall_YP_Raksha.bat installer\staging\Uninstall_YP_Raksha.bat >nul
if errorlevel 1 (
  echo ERROR: could not stage the secure uninstaller.
  exit /b 1
)
if exist release\INSTALL_WINDOWS.txt copy /Y release\INSTALL_WINDOWS.txt installer\staging\INSTALL_WINDOWS.txt >nul
if exist release\VERSION.txt copy /Y release\VERSION.txt installer\staging\VERSION.txt >nul
if exist installer\staging\extension rmdir /S /Q installer\staging\extension >nul 2>&1

echo.
echo ============================================================
echo  3) Compiling Setup EXE (Inno Setup)
echo ============================================================
if not exist installer\output mkdir installer\output
if exist installer\output\YP_Raksha_Setup.exe del /F /Q installer\output\YP_Raksha_Setup.exe
"%ISCC%" installer\YP_Raksha.iss
if errorlevel 1 (
  echo Inno compile failed.
  exit /b 1
)
if not exist installer\output\YP_Raksha_Setup.exe (
  echo ERROR: Inno Setup reported success but produced no YP_Raksha_Setup.exe.
  exit /b 1
)
copy /Y installer\output\YP_Raksha_Setup.exe release\YP_Raksha_Setup.exe >nul
if errorlevel 1 (
  echo ERROR: could not publish release\YP_Raksha_Setup.exe.
  exit /b 1
)
if exist release\Gateway_Guard_Setup.exe del /F /Q release\Gateway_Guard_Setup.exe

echo.
echo ============================================================
echo  4) Creating Windows employee ZIP (YP_Raksha_Windows.zip)
echo ============================================================
if exist release\YP_Raksha_Windows.zip del /F /Q release\YP_Raksha_Windows.zip
if exist release\Gateway_Guard_Windows.zip del /F /Q release\Gateway_Guard_Windows.zip

rem -- Stage the Setup EXE + portable EXE into a temp zip folder
set ZIP_STAGE=installer\staging-win-zip
if exist "%ZIP_STAGE%" rmdir /S /Q "%ZIP_STAGE%"
mkdir "%ZIP_STAGE%"

if exist release\YP_Raksha_Setup.exe copy /Y release\YP_Raksha_Setup.exe "%ZIP_STAGE%\YP_Raksha_Setup.exe" >nul
if not exist "%ZIP_STAGE%\portable" mkdir "%ZIP_STAGE%\portable"
if exist build\exe\YP_Raksha.exe (
  copy /Y build\exe\YP_Raksha.exe "%ZIP_STAGE%\portable\YP_Raksha.exe" >nul
) else if exist release\YP_Raksha.exe (
  copy /Y release\YP_Raksha.exe "%ZIP_STAGE%\portable\YP_Raksha.exe" >nul
) else if exist release\Gateway_Guard.exe (
  copy /Y release\Gateway_Guard.exe "%ZIP_STAGE%\portable\YP_Raksha.exe" >nul
)
copy /Y release\gateway_guard_config.json "%ZIP_STAGE%\gateway_guard_config.json" >nul
if exist release\INSTALL_WINDOWS.txt copy /Y release\INSTALL_WINDOWS.txt "%ZIP_STAGE%\INSTALL_WINDOWS.txt" >nul
if exist installer\EMPLOYEE_README.txt copy /Y installer\EMPLOYEE_README.txt "%ZIP_STAGE%\EMPLOYEE_README.txt" >nul
if exist release\Uninstall_YP_Raksha.bat copy /Y release\Uninstall_YP_Raksha.bat "%ZIP_STAGE%\Uninstall_YP_Raksha.bat" >nul
if exist release\Update_YP_Raksha.ps1 copy /Y release\Update_YP_Raksha.ps1 "%ZIP_STAGE%\Update_YP_Raksha.ps1" >nul
if exist release\VERSION.txt copy /Y release\VERSION.txt "%ZIP_STAGE%\VERSION.txt" >nul

rem -- Use PowerShell Compress-Archive (Windows 10+) to build the ZIP
powershell -NoProfile -Command "Compress-Archive -Path '%ZIP_STAGE%\*' -DestinationPath 'release\YP_Raksha_Windows.zip' -Force; if (Test-Path 'release\Gateway_Guard_Windows.zip') { Remove-Item -Force 'release\Gateway_Guard_Windows.zip' }"
if errorlevel 1 (
  echo WARNING: PowerShell Compress-Archive failed — Windows ZIP not created.
) else (
  echo Windows ZIP: release\YP_Raksha_Windows.zip
)
rmdir /S /Q "%ZIP_STAGE%"

echo.
echo ============================================================
echo  SUCCESS — YP Raksha 1.1
echo  Employee installer (Setup EXE):
echo    release\YP_Raksha_Setup.exe
echo  Employee download package (Windows ZIP):
echo    release\YP_Raksha_Windows.zip
echo ============================================================
if exist release\YP_Raksha_Windows.zip (
  dir release\YP_Raksha_Windows.zip
)
if exist release\YP_Raksha_Setup.exe dir release\YP_Raksha_Setup.exe
if exist release\YP_Raksha.exe dir release\YP_Raksha.exe
endlocal
