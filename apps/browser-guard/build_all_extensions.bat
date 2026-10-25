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
echo  2/2: Packaging Chromium extensions; Firefox requires a Mozilla-signed XPI...
echo ============================================================
python scripts\pack_all_extensions.py
if errorlevel 1 (
  echo [ERROR] Extension packing failed
  pause
  exit /b 1
)

echo.
echo ============================================================
echo  SUCCESS: Chromium extension packages built.
echo  - Unpacked ZIP : %~dp0release\yp-raksha.zip
echo  - Chrome CRX   : %~dp0release\yp-raksha.crx
if exist "%~dp0release\yp-raksha.xpi" (
  echo  - Firefox XPI  : %~dp0release\yp-raksha.xpi ^(Mozilla-signed^)
) else (
  echo  - Firefox XPI  : not emitted; provide a current Mozilla-signed package using GATEWAY_GUARD_SIGNED_XPI
  echo  - Dev only      : %~dp0release\yp-raksha-unsigned.xpi
)
echo ============================================================
pause
