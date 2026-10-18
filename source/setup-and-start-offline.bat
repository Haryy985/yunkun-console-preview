@echo off
setlocal
cd /d "%~dp0"
echo 云鲲自动化渗透控制台 - 离线启动
powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0setup-and-start-offline.ps1" %*
set "result=%errorlevel%"
if not "%result%"=="0" (
  echo.
  echo 离线启动失败，请阅读上面的错误信息。
  pause
)
exit /b %result%
