@echo off
setlocal
cd /d "%~dp0"
powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0setup-and-start.ps1" -Stop
set "result=%errorlevel%"
pause
exit /b %result%
