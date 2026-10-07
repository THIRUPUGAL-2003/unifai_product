@echo off
title Gateway Enterprise Security Guard Agent
echo ====================================================
echo   Launching Gateway Security Guard Agent...
echo ====================================================
python "%~dp0..\..\apps\browser-guard\agent\gateway_agent.py"
pause
