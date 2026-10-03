@echo off
setlocal EnableDelayedExpansion
echo ====================================================
echo Starting Raksha ^& AI Guard Proxy Launcher
echo ====================================================

set "REPO_ROOT=%~dp0..\.."
set "PROXY_SCRIPT=%REPO_ROOT%\apps\browser-guard\proxy\browser_ai_proxy.py"
set "ENV_FILE=%REPO_ROOT%\.env"

if not exist "%ENV_FILE%" (
  echo ERROR: .env not found at %ENV_FILE%
  echo Set RAKSHA_PROXY_ADDR, APP_PORT, and MITM_WEB_PORT in .env
  exit /b 1
)

for /f "usebackq tokens=1,* delims==" %%A in (`findstr /B /I /C:"RAKSHA_PROXY_ADDR=" /C:"APP_PORT=" /C:"MITM_WEB_PORT=" "%ENV_FILE%"`) do (
  set "%%A=%%B"
)

if not defined RAKSHA_PROXY_ADDR (
  echo ERROR: RAKSHA_PROXY_ADDR missing in .env
  exit /b 1
)
if not defined APP_PORT (
  echo ERROR: APP_PORT missing in .env
  exit /b 1
)
if not defined MITM_WEB_PORT (
  echo ERROR: MITM_WEB_PORT missing in .env
  exit /b 1
)

for /f "tokens=2 delims=:" %%P in ("%RAKSHA_PROXY_ADDR%") do set "PROXY_PORT_ONLY=%%P"
if not defined PROXY_PORT_ONLY (
  echo ERROR: RAKSHA_PROXY_ADDR must look like 127.0.0.1:PORT
  exit /b 1
)

echo 1. Starting Proxy Interceptor on %RAKSHA_PROXY_ADDR% ...
start /b mitmweb -p %PROXY_PORT_ONLY% --web-port %MITM_WEB_PORT% -s "%PROXY_SCRIPT%" --set block_global=false

timeout /t 2 >nul

echo 2. Opening Chrome with Proxy...
start "" "C:\Program Files\Google\Chrome\Application\chrome.exe" --proxy-server="http://%RAKSHA_PROXY_ADDR%" --user-data-dir="%TEMP%\chrome_proxy" --ignore-certificate-errors "http://localhost:%APP_PORT%/workspace/browser-ai" "http://127.0.0.1:%MITM_WEB_PORT%"

echo ====================================================
echo Everything is running ^& opened in Chrome!
echo - Dashboard UI: http://localhost:%APP_PORT%/workspace/browser-ai
echo - Proxy: %RAKSHA_PROXY_ADDR%
echo - Proxy Monitor UI: http://127.0.0.1:%MITM_WEB_PORT%
echo ====================================================
endlocal
