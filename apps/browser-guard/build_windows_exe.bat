@echo off
setlocal EnableExtensions
title Build YP Raksha Windows Standalone Executable

cd /d "%~dp0"

echo ============================================================
echo  1/3: Syncing configuration from root .env...
echo ============================================================
python scripts\sync_config_from_env.py
if errorlevel 1 (
  echo [ERROR] Failed to sync config from .env
  pause
  exit /b 1
)

echo.
echo ============================================================
echo  2/3: Encrypting internal proxy runtime bundle...
echo ============================================================
python installer\encrypt_proxy_bundle.py
if errorlevel 1 (
  echo [ERROR] Failed to encrypt proxy bundle
  pause
  exit /b 1
)

echo.
echo ============================================================
echo  3/3: Building standalone YP_Raksha.exe (Zero Setup Wizard)...
echo ============================================================
python installer\build_agent.py
if errorlevel 1 (
  echo [ERROR] PyInstaller build failed
  pause
  exit /b 1
)

echo.
echo ============================================================
echo  SUCCESS: YP_Raksha.exe built cleanly!
echo  Location: %~dp0release\YP_Raksha.exe
echo ============================================================
pause
