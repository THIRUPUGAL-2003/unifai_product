@echo off
title Raksha Enterprise Security Guard Agent
echo ====================================================
echo   Launching Raksha Security Guard Agent...
echo ====================================================
python "%~dp0..\..\apps\browser-guard\agent\raksha_agent.py"
pause
