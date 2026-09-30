@echo off
title Stop UnifAI Security Proxy
echo ====================================================
echo   Disabling Windows System Proxy ^& Stopping Agent...
echo ====================================================

rem Disable Windows System Proxy in Registry
reg add "HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings" /v ProxyEnable /t REG_DWORD /d 0 /f >nul 2>&1

rem Terminate any running mitmweb or mitmdump background processes
taskkill /F /IM mitmdump.exe >nul 2>&1
taskkill /F /IM mitmweb.exe >nul 2>&1
taskkill /F /IM UnifAI_Guard.exe >nul 2>&1

echo ====================================================
echo SUCCESS: Windows System Proxy disabled!
echo Normal internet browsing restored.
echo ====================================================
pause
