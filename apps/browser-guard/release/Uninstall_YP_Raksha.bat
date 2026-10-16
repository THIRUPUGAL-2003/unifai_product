@echo off
setlocal
echo ========================================================
echo        YP Raksha Enterprise Uninstaller
echo ========================================================
echo.
echo YP Raksha is enterprise security software.
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

set "GUARD_EXE="
for %%F in (
    "%LOCALAPPDATA%\Programs\YP Raksha\YP_Raksha.exe"
    "%LOCALAPPDATA%\Programs\Gateway\Guard\YP_Raksha.exe"
    "%LOCALAPPDATA%\Programs\Gateway\Guard\Gateway_Guard.exe"
    "%~dp0YP_Raksha.exe"
    "%~dp0portable\YP_Raksha.exe"
    "%~dp0Gateway_Guard.exe"
    "%~dp0portable\Gateway_Guard.exe"
) do (
    if exist "%%~F" (
        if not defined GUARD_EXE set "GUARD_EXE=%%~F"
    )
)

if not defined GUARD_EXE (
    echo [ERROR] YP_Raksha.exe not found.
    pause
    exit /b 1
)

echo Verifying key with security server...
"%GUARD_EXE%" --uninstall "%KEY%"
set EXIT_CODE=%ERRORLEVEL%

if %EXIT_CODE% EQU 0 (
    echo.
    echo [SUCCESS] YP Raksha has been uninstalled successfully.
) else (
    echo.
    echo [FAILED] Uninstall failed (code %EXIT_CODE%).
    echo Please verify the uninstall key and ensure the backend server is reachable.
)

pause
exit /b %EXIT_CODE%
