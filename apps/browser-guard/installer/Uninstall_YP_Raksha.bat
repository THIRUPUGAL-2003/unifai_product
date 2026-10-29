@echo off
setlocal EnableExtensions
echo ========================================================
echo        YP Raksha Enterprise Uninstaller
echo ========================================================
echo.
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

if "%~1"=="" (
    echo A secure prompt will ask for the uninstall key.
    "%GUARD_EXE%" --uninstall-prompt
) else (
    "%GUARD_EXE%" --uninstall "%~1"
)
set EXIT_CODE=%ERRORLEVEL%

if "%EXIT_CODE%"=="0" (
    echo.
    echo [INFO] Removing browser policies and proxy configuration...
    reg add "HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings" /v AutoConfigURL /t REG_SZ /d "" /f >nul 2>&1
    reg add "HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings" /v ProxyEnable /t REG_DWORD /d 0 /f >nul 2>&1
    reg delete "HKCU\Software\Policies\Google\Chrome\ExtensionInstallForcelist" /f >nul 2>&1
    reg delete "HKCU\Software\Policies\Google\Chrome\ExtensionSettings" /f >nul 2>&1
    reg delete "HKCU\Software\Policies\Google\Chrome\ExtensionInstallSources" /f >nul 2>&1
    reg delete "HKCU\Software\Policies\Microsoft\Edge\ExtensionInstallForcelist" /f >nul 2>&1
    reg delete "HKCU\Software\Policies\Microsoft\Edge\ExtensionSettings" /f >nul 2>&1
    reg delete "HKCU\Software\Policies\Microsoft\Edge\ExtensionInstallSources" /f >nul 2>&1
    reg delete "HKCU\Software\Policies\BraveSoftware\Brave\ExtensionInstallForcelist" /f >nul 2>&1
    reg delete "HKCU\Software\Policies\BraveSoftware\Brave\ExtensionSettings" /f >nul 2>&1
    reg delete "HKCU\Software\Policies\Mozilla\Firefox\ExtensionSettings" /f >nul 2>&1
    echo.
    echo [SUCCESS] YP Raksha has been uninstalled successfully.
) else (
    if "%EXIT_CODE%"=="3" (
        echo.
        echo [INFO] Uninstall cancelled. YP Raksha remains installed.
    ) else (
        echo.
        echo [FAILED] Uninstall failed (code %EXIT_CODE%).
        echo YP Raksha remains installed. Check the key and network connection.
    )
)

pause
exit /b %EXIT_CODE%
