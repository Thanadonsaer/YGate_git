@echo off
setlocal EnableExtensions
cd /d "%~dp0"

if not exist ".cache" mkdir ".cache"
if not exist ".cache\go-build" mkdir ".cache\go-build"
if not exist ".cache\go-mod" mkdir ".cache\go-mod"

set "GOCACHE=%CD%\.cache\go-build"
set "GOMODCACHE=%CD%\.cache\go-mod"
set "CGO_ENABLED=0"

echo Starting CHPP middleware dev runner...
echo Web UI: http://127.0.0.1:8081
echo.

go run .\cmd\middleware -run -db middleware.db -listen 0.0.0.0:8081 %*
exit /b %ERRORLEVEL%
