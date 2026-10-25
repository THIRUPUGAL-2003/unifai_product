@echo off
setlocal EnableExtensions
title Build All YP-Raksha Browser Extensions

cd /d "%~dp0"

echo ============================================================
echo  1/2: Syncing extension configuration from .env...
echo ============================================================
python scripts\sync_config_from_env.py
if errorlevel 1 (
  echo [ERROR] Failed to sync config from .env
  pause
  exit /b 1
)

echo.
echo ============================================================
echo  2/2: Packaging extensions for Chrome, Brave, Edge, Opera, Firefox...
echo ============================================================
python scripts\pack_all_extensions.py
if errorlevel 1 (
  echo [ERROR] Extension packing failed
  pause
  exit /b 1
)

echo.
echo ============================================================
echo  SUCCESS: All browser extensions built cleanly!
echo  - Unpacked ZIP : %~dp0release\yp-raksha.zip
echo  - Chrome CRX   : %~dp0release\yp-raksha.crx
echo  - Firefox XPI  : %~dp0release\yp-raksha.xpi
echo ============================================================
pause
