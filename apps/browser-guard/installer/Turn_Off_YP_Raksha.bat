@echo off
setlocal EnableExtensions
title Turn OFF YP Raksha Guard Protection

:: 0. Check for Administrator privileges, auto-elevate if needed
net session >nul 2>&1
if %errorLevel% neq 0 (
    echo Requesting Administrator privileges to reset system and browser policies...
    powershell -NoProfile -Command "Start-Process cmd -ArgumentList '/c \"\"%~f0\"\"' -Verb RunAs"
    exit /b
)

echo ========================================================
echo        Turn OFF YP Raksha Guard Protection
echo ========================================================
echo.
echo 1. Stopping YP Raksha background processes...
taskkill /F /IM YP_Raksha.exe >nul 2>&1
taskkill /F /IM Gateway_Guard.exe >nul 2>&1

echo 2. Disabling Windows system proxy and PAC (HKCU and HKLM)...
reg add "HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings" /v ProxyEnable /t REG_DWORD /d 0 /f >nul 2>&1
reg delete "HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings" /v AutoConfigURL /f >nul 2>&1
reg delete "HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings" /v ProxyServer /f >nul 2>&1

reg add "HKLM\Software\Microsoft\Windows\CurrentVersion\Internet Settings" /v ProxyEnable /t REG_DWORD /d 0 /f >nul 2>&1
reg delete "HKLM\Software\Microsoft\Windows\CurrentVersion\Internet Settings" /v AutoConfigURL /f >nul 2>&1
reg delete "HKLM\Software\Microsoft\Windows\CurrentVersion\Internet Settings" /v ProxyServer /f >nul 2>&1

echo 3. Clearing browser managed proxy policies across HKCU and HKLM...
for %%R in ("HKCU" "HKLM") do (
    for %%P in (
        "Software\Policies\Google\Chrome"
        "Software\Policies\Google\Chrome Beta"
        "Software\Policies\Google\Chrome Dev"
        "Software\Policies\Microsoft\Edge"
        "Software\Policies\Microsoft\Edge Beta"
        "Software\Policies\BraveSoftware\Brave"
        "Software\Policies\Opera Software\Opera"
        "Software\Google\Chrome"
        "Software\Microsoft\Edge"
    ) do (
        reg delete "%%~R\%%~P" /v ProxyMode /f >nul 2>&1
        reg delete "%%~R\%%~P" /v ProxyPacUrl /f >nul 2>&1
        reg delete "%%~R\%%~P" /v ProxyServer /f >nul 2>&1
        reg delete "%%~R\%%~P" /v ProxyBypassList /f >nul 2>&1
        reg delete "%%~R\%%~P" /v DnsOverHttpsMode /f >nul 2>&1
        reg delete "%%~R\%%~P" /v QuicAllowed /f >nul 2>&1
        reg delete "%%~R\%%~P" /v ExtensionSettings /f >nul 2>&1
        reg delete "%%~R\%%~P\ExtensionInstallForcelist" /f >nul 2>&1
    )
)

echo 4. Purging extension cache and browser storage...
for %%B in ("Google\Chrome" "Microsoft\Edge" "BraveSoftware\Brave-Browser") do (
    for /d %%U in ("%LOCALAPPDATA%\%%~B\User Data\*") do (
        if exist "%%~U\Local Extension Settings\neiemlideblokbpmnjblipmhgkklfjbd" rmdir /s /q "%%~U\Local Extension Settings\neiemlideblokbpmnjblipmhgkklfjbd" >nul 2>&1
        if exist "%%~U\Sync Extension Settings\neiemlideblokbpmnjblipmhgkklfjbd" rmdir /s /q "%%~U\Sync Extension Settings\neiemlideblokbpmnjblipmhgkklfjbd" >nul 2>&1
        for /d %%I in ("%%~U\IndexedDB\chrome-extension_neiemlideblokbpmnjblipmhgkklfjbd*") do rmdir /s /q "%%~I" >nul 2>&1
    )
)
if exist "%LOCALAPPDATA%\Gateway\Guard\chrome_storage_cache.json" del /f /q "%LOCALAPPDATA%\Gateway\Guard\chrome_storage_cache.json" >nul 2>&1
if exist "%LOCALAPPDATA%\Gateway\Guard\targets_cache.json" del /f /q "%LOCALAPPDATA%\Gateway\Guard\targets_cache.json" >nul 2>&1
if exist "%LOCALAPPDATA%\Gateway\Guard\rules_cache.json" del /f /q "%LOCALAPPDATA%\Gateway\Guard\rules_cache.json" >nul 2>&1
if exist "%LOCALAPPDATA%\Gateway\Guard\recent_extension_evals.json" del /f /q "%LOCALAPPDATA%\Gateway\Guard\recent_extension_evals.json" >nul 2>&1
if exist "%LOCALAPPDATA%\Gateway\Guard\proxy.pac" del /f /q "%LOCALAPPDATA%\Gateway\Guard\proxy.pac" >nul 2>&1

echo 5. Cleaning Desktop AI environment variables...
reg delete "HKCU\Environment" /v NODE_EXTRA_CA_CERTS /f >nul 2>&1
reg delete "HKCU\Environment" /v REQUESTS_CA_BUNDLE /f >nul 2>&1
reg delete "HKCU\Environment" /v SSL_CERT_FILE /f >nul 2>&1
reg delete "HKCU\Environment" /v CURL_CA_BUNDLE /f >nul 2>&1

echo 6. Refreshing Windows Internet Settings...
powershell -NoProfile -Command "try { [ctypes] = Add-Type -MemberDefinition '[DllImport(\"wininet.dll\")] public static extern bool InternetSetOption(int h, int opt, int val, int len);' -Name 'WinINetOpt' -Namespace 'WinINet' -PassThru; [WinINet.WinINetOpt]::InternetSetOption(0, 39, 0, 0); [WinINet.WinINetOpt]::InternetSetOption(0, 37, 0, 0) } catch {}" >nul 2>&1

echo.
echo ========================================================
echo  [SUCCESS] YP Raksha Guard has been turned OFF.
echo  Browsers now connect directly to the internet without proxy.
echo ========================================================
timeout /t 3 >nul 2>&1

