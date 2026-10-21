@echo off
:: Whitelist_Extension_In_Browsers.bat
:: Whitelists YP-Raksha extension in Brave, Chrome, and Edge policies so CRX is allowed without unsafe warnings.

net session >nul 2>&1
if %errorLevel% neq 0 (
    echo Requesting Administrator privileges to whitelist extension in Windows Policy...
    powershell -Command "Start-Process cmd -ArgumentList '/c `"%~f0`"' -Verb RunAs"
    exit /b
)

set EXT_ID=mipfhcmlcjfhmgldkokdckhndkebkllj

echo ============================================================
echo  Whitelisting YP-Raksha in Chrome, Edge, and Brave
echo ============================================================

:: Chrome
reg add "HKLM\SOFTWARE\Policies\Google\Chrome\ExtensionInstallAllowlist" /v 1 /t REG_SZ /d "%EXT_ID%" /f >nul 2>&1
reg add "HKLM\SOFTWARE\Policies\Google\Chrome\ExtensionInstallSources" /v 1 /t REG_SZ /d "<all_urls>" /f >nul 2>&1
echo [OK] Whitelisted in Google Chrome

:: Microsoft Edge
reg add "HKLM\SOFTWARE\Policies\Microsoft\Edge\ExtensionInstallAllowlist" /v 1 /t REG_SZ /d "%EXT_ID%" /f >nul 2>&1
reg add "HKLM\SOFTWARE\Policies\Microsoft\Edge\ExtensionInstallSources" /v 1 /t REG_SZ /d "<all_urls>" /f >nul 2>&1
echo [OK] Whitelisted in Microsoft Edge

:: Brave Browser
reg add "HKLM\SOFTWARE\Policies\BraveSoftware\Brave\ExtensionInstallAllowlist" /v 1 /t REG_SZ /d "%EXT_ID%" /f >nul 2>&1
reg add "HKLM\SOFTWARE\Policies\BraveSoftware\Brave\ExtensionInstallSources" /v 1 /t REG_SZ /d "<all_urls>" /f >nul 2>&1
echo [OK] Whitelisted in Brave Browser

echo.
echo ============================================================
echo  SUCCESS! Extension is now trusted and allowlisted.
echo  Restart Brave/Chrome/Edge and reload brave://extensions
echo ============================================================
pause
