@echo off
setlocal EnableExtensions
title Build YP-Raksha Master Deployment Suite

cd /d "%~dp0"

echo ============================================================
echo  Building YP-Raksha Decoupled Master Suite from .env
echo ============================================================
python scripts\build_master_suite.py
if errorlevel 1 (
  echo [ERROR] Suite packaging failed.
  pause
  exit /b 1
)

echo.
echo ============================================================
echo  SUCCESS: Master Suite generated!
echo  Location: %~dp0release\YP_Raksha_Decoupled_Suite.zip
echo ============================================================
pause
