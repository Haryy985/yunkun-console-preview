@echo off
setlocal
cd /d "%~dp0"
echo Yunkun - native setup and startup
powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0setup-and-start.ps1" %*
set "result=%errorlevel%"
if not "%result%"=="0" (
  echo.
  echo Setup did not complete. Read the error above. Existing data was not reset.
  pause
)
exit /b %result%
