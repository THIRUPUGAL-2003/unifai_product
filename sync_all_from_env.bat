@echo off
title UnifAI Master Environment Synchronizer
cd /d "%~dp0"
echo ============================================================
echo   UnifAI Enterprise - Syncing all components from .env
echo ============================================================
python sync_all_from_env.py %*
if %ERRORLEVEL% equ 0 (
    echo.
    echo [SUCCESS] Everything synchronized successfully!
) else (
    echo.
    echo [ERROR] Synchronization encountered an issue.
)
pause
