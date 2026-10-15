@echo off
setlocal
echo ========================================================
echo        Gateway Guard Enterprise Uninstaller
echo ========================================================
echo.
echo Gateway Guard is enterprise security software.
echo A valid Uninstall Key (Company Key or Guard Key) is required.
echo Unauthorized deletion or tampering is strictly monitored.
echo.
set /p KEY="Enter Uninstall Key: "
if "%KEY%"=="" (
    echo.
    echo [ERROR] Uninstall key is required. Uninstall aborted.
    pause
    exit /b 1
)

set "GUARD_EXE=%LOCALAPPDATA%\Programs\Gateway\Guard\Gateway_Guard.exe"
if not exist "%GUARD_EXE%" (
    if exist "%~dp0Gateway_Guard.exe" set "GUARD_EXE=%~dp0Gateway_Guard.exe"
)
if not exist "%GUARD_EXE%" (
    if exist "%~dp0portable\Gateway_Guard.exe" set "GUARD_EXE=%~dp0portable\Gateway_Guard.exe"
)

if not exist "%GUARD_EXE%" (
    echo [ERROR] Gateway_Guard.exe not found.
    pause
    exit /b 1
)

echo Verifying key with security server...
"%GUARD_EXE%" --uninstall "%KEY%"
set EXIT_CODE=%ERRORLEVEL%

if %EXIT_CODE% EQU 0 (
    echo.
    echo [SUCCESS] Gateway Guard has been uninstalled successfully.
) else (
    echo.
    echo [ERROR] Invalid uninstall key! Uninstall rejected.
)
pause
exit /b %EXIT_CODE%
