@echo off
setlocal
cd /d "%~dp0"
echo ========================================================
echo   UnifAI / Gateway - Sync All Configs from .env
echo ========================================================
echo.
python sync_all_from_env.py %*
if %ERRORLEVEL% NEQ 0 (
    echo.
    echo [ERROR] Synchronization encountered an issue!
) else (
    echo.
    echo [DONE] Environment synchronization completed successfully.
)
echo.
if "%~1"=="" pause
exit /b %ERRORLEVEL%
