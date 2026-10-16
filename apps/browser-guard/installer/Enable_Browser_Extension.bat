@echo off
:: Enable_Browser_Extension.bat
:: Enterprise registration for Gateway Guard (YP-Raksha) in Google Chrome, Microsoft Edge, and Brave

net session >nul 2>&1
if %errorLevel% neq 0 (
    echo Requesting Administrator privileges to register Enterprise Extension Policy...
    powershell -Command "Start-Process cmd -ArgumentList '/c `"%~f0`"' -Verb RunAs"
    exit /b
)

echo ============================================================
echo  Registering Enterprise Extension Policies (HKLM)
echo ============================================================

set EXT_ID=mipfhcmlcjfhmgldkokdckhndkebkllj
set PAC_PORT=
if defined PAC_HTTP_PORT set PAC_PORT=%PAC_HTTP_PORT%
if exist "%~dp0..\..\..\.env" (
    for /f "tokens=2 delims==" %%P in ('findstr /i "PAC_HTTP_PORT=" "%~dp0..\..\..\.env"') do set PAC_PORT=%%P
)
if not defined PAC_PORT (
    echo ERROR: PAC_HTTP_PORT is not defined in .env!
    exit /b 1
)
set UPDATE_URL=http://127.0.0.1:%PAC_PORT%/extension/update.xml
set FORCE_VAL=%EXT_ID%;%UPDATE_URL%
set SETTINGS_JSON={\"%EXT_ID%\":{\"installation_mode\":\"force_installed\",\"update_url\":\"%UPDATE_URL%\",\"incognito\":\"spanning\",\"toolbar_pin\":\"force_unpinned\"}}

:: Chrome
reg add "HKLM\SOFTWARE\Policies\Google\Chrome" /v ExtensionSettings /t REG_SZ /d "%SETTINGS_JSON%" /f >nul
reg add "HKLM\SOFTWARE\Policies\Google\Chrome\ExtensionInstallForcelist" /v 1 /t REG_SZ /d "%FORCE_VAL%" /f >nul
reg add "HKLM\SOFTWARE\Policies\Google\Chrome\ExtensionInstallAllowlist" /v 1 /t REG_SZ /d "%EXT_ID%" /f >nul
reg add "HKLM\SOFTWARE\Policies\Google\Chrome\ExtensionInstallSources" /v 1 /t REG_SZ /d "http://127.0.0.1:%PAC_PORT%/*" /f >nul
reg add "HKLM\SOFTWARE\Policies\Google\Chrome\ExtensionInstallSources" /v 2 /t REG_SZ /d "http://localhost:%PAC_PORT%/*" /f >nul
reg add "HKLM\SOFTWARE\Policies\Google\Chrome\ExtensionInstallSources" /v 3 /t REG_SZ /d "https://*/*" /f >nul
echo [OK] Registered Google Chrome Enterprise Policy

:: Microsoft Edge
reg add "HKLM\SOFTWARE\Policies\Microsoft\Edge" /v ExtensionSettings /t REG_SZ /d "%SETTINGS_JSON%" /f >nul
reg add "HKLM\SOFTWARE\Policies\Microsoft\Edge\ExtensionInstallForcelist" /v 1 /t REG_SZ /d "%FORCE_VAL%" /f >nul
reg add "HKLM\SOFTWARE\Policies\Microsoft\Edge\ExtensionInstallAllowlist" /v 1 /t REG_SZ /d "%EXT_ID%" /f >nul
reg add "HKLM\SOFTWARE\Policies\Microsoft\Edge\ExtensionInstallSources" /v 1 /t REG_SZ /d "http://127.0.0.1:%PAC_PORT%/*" /f >nul
reg add "HKLM\SOFTWARE\Policies\Microsoft\Edge\ExtensionInstallSources" /v 2 /t REG_SZ /d "http://localhost:%PAC_PORT%/*" /f >nul
reg add "HKLM\SOFTWARE\Policies\Microsoft\Edge\ExtensionInstallSources" /v 3 /t REG_SZ /d "https://*/*" /f >nul
echo [OK] Registered Microsoft Edge Enterprise Policy

:: Brave
reg add "HKLM\SOFTWARE\Policies\BraveSoftware\Brave" /v ExtensionSettings /t REG_SZ /d "%SETTINGS_JSON%" /f >nul
reg add "HKLM\SOFTWARE\Policies\BraveSoftware\Brave\ExtensionInstallForcelist" /v 1 /t REG_SZ /d "%FORCE_VAL%" /f >nul
reg add "HKLM\SOFTWARE\Policies\BraveSoftware\Brave\ExtensionInstallAllowlist" /v 1 /t REG_SZ /d "%EXT_ID%" /f >nul
reg add "HKLM\SOFTWARE\Policies\BraveSoftware\Brave\ExtensionInstallSources" /v 1 /t REG_SZ /d "http://127.0.0.1:%PAC_PORT%/*" /f >nul
reg add "HKLM\SOFTWARE\Policies\BraveSoftware\Brave\ExtensionInstallSources" /v 2 /t REG_SZ /d "http://localhost:%PAC_PORT%/*" /f >nul
reg add "HKLM\SOFTWARE\Policies\BraveSoftware\Brave\ExtensionInstallSources" /v 3 /t REG_SZ /d "https://*/*" /f >nul
echo [OK] Registered Brave Browser Enterprise Policy

:: Remove external CRX keys and stale HKCU force-install policies
reg delete "HKCU\Software\Google\Chrome\Extensions\%EXT_ID%" /f >nul 2>&1
reg delete "HKCU\Software\Microsoft\Edge\Extensions\%EXT_ID%" /f >nul 2>&1
reg delete "HKCU\Software\BraveSoftware\Brave\Extensions\%EXT_ID%" /f >nul 2>&1
reg delete "HKCU\SOFTWARE\Policies\Google\Chrome\ExtensionInstallForcelist" /f >nul 2>&1
reg delete "HKCU\SOFTWARE\Policies\Google\Chrome" /v ExtensionSettings /f >nul 2>&1
reg delete "HKCU\SOFTWARE\Policies\Microsoft\Edge\ExtensionInstallForcelist" /f >nul 2>&1
reg delete "HKCU\SOFTWARE\Policies\Microsoft\Edge" /v ExtensionSettings /f >nul 2>&1

echo.
echo ============================================================
echo  SUCCESS! Enterprise Browser Policies Registered.
echo  Restart Google Chrome, Edge, and Brave to see YP-Raksha
echo  force-installed and fully enabled without warnings!
echo ============================================================
timeout /t 3 >nul
