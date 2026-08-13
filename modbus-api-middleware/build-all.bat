@echo off
setlocal EnableExtensions
cd /d "%~dp0"

if not exist "build" mkdir "build"
del /q "build\*.exe" "build\*.exe~" "build\patches\*.zip" >nul 2>nul
for /d %%D in ("build\*") do rmdir /s /q "%%D" >nul 2>nul
if exist ".gocache" rmdir /s /q ".gocache"
if not exist ".cache" mkdir ".cache"
if not exist ".cache\go-build" mkdir ".cache\go-build"
if not exist ".cache\go-mod" mkdir ".cache\go-mod"
del /q ".cache\*.bat" ".cache\*.json" ".cache\*.env" ".cache\*.exe" ".cache\*.db" 2>nul

if not exist "build\linux\build" mkdir "build\linux\build"
if not exist "build\linux\deploy" mkdir "build\linux\deploy"
if not exist "build\patches" mkdir "build\patches"

set "GOCACHE=%CD%\.cache\go-build"
set "GOMODCACHE=%CD%\.cache\go-mod"
set "CGO_ENABLED=0"
set "GOOS="
set "GOARCH="
set "GOARM="
set "VERSION=0.2.l"
set "BOOTSTRAP_VERSION=%VERSION%-bootstrap"
set "KEYS_FILE=%CD%\license-keys.env"

if exist "%KEYS_FILE%" (
  echo Loading license keys: %KEYS_FILE%d
  for /f "usebackq eol=# tokens=1,* delims==" %%A in ("%KEYS_FILE%") do if not "%%A"=="" set "%%A=%%B"
)
if not defined CHPP_LICENSE_PUBLIC_KEY (
  if not defined CHPP_LICENSE_KEY_NAME set "CHPP_LICENSE_KEY_NAME=DEFAULT"
  call set "CHPP_LICENSE_PUBLIC_KEY=%%CHPP_LICENSE_PUBLIC_KEY_%CHPP_LICENSE_KEY_NAME%%%"
)
if not defined CHPP_LICENSE_PUBLIC_KEY (
  echo Missing license public key.
  echo Set CHPP_LICENSE_PUBLIC_KEY or create license-keys.env from license-keys.env.example.
  goto error
)
set "LDFLAGS=-s -w -X main.version=%VERSION% -X main.licensePublicKey=%CHPP_LICENSE_PUBLIC_KEY%"
set "BOOTSTRAP_LDFLAGS=-s -w -X main.version=%BOOTSTRAP_VERSION% -X main.licensePublicKey=%CHPP_LICENSE_PUBLIC_KEY%"

echo Version: %VERSION%
if defined CHPP_LICENSE_KEY_NAME echo License key: %CHPP_LICENSE_KEY_NAME%

echo [1/4] Test
go test ./... || goto error

echo [2/4] Vet
go vet ./... || goto error

echo [3/4] Build Windows single EXE
set "GOOS=windows"
set "GOARCH=amd64"
set "GOARM="
go build -trimpath -ldflags "%LDFLAGS%" -o "build\middleware-v%VERSION%-windows-amd64.exe" .\cmd\middleware || goto error
powershell -NoProfile -ExecutionPolicy Bypass -File "deploy\make-update-zip.ps1" -Version "%VERSION%" -TargetOS "windows" -TargetArch "amd64" -Binary "build\middleware-v%VERSION%-windows-amd64.exe" -Out "build\patches\chpp-middleware-v%VERSION%-windows-amd64-update.zip" || goto error
rem Feature-only bridge for legacy clients whose downloader has a hard 60-second timeout.
go build -trimpath -gcflags "all=-l" -ldflags "%BOOTSTRAP_LDFLAGS%" -o "build\middleware-v%BOOTSTRAP_VERSION%-windows-amd64.exe" .\cmd\update-bridge || goto error
powershell -NoProfile -ExecutionPolicy Bypass -File "deploy\make-update-zip.ps1" -Version "%BOOTSTRAP_VERSION%" -TargetOS "windows" -TargetArch "amd64" -Binary "build\middleware-v%BOOTSTRAP_VERSION%-windows-amd64.exe" -Out "build\patches\chpp-middleware-v%BOOTSTRAP_VERSION%-windows-amd64-update.zip" || goto error

echo [4/4] Build Linux Debian amd64 package
set "GOOS=linux"
set "GOARCH=amd64"
set "GOARM="
go build -trimpath -ldflags "%LDFLAGS%" -o "build\linux\build\middleware-linux-amd64" .\cmd\middleware || goto error
powershell -NoProfile -ExecutionPolicy Bypass -File "deploy\make-update-zip.ps1" -Version "%VERSION%" -TargetOS "linux" -TargetArch "amd64" -Binary "build\linux\build\middleware-linux-amd64" -Out "build\patches\chpp-middleware-v%VERSION%-linux-amd64-update.zip" || goto error
go build -trimpath -gcflags "all=-l" -ldflags "%BOOTSTRAP_LDFLAGS%" -o "build\linux\build\middleware-v%BOOTSTRAP_VERSION%-linux-amd64" .\cmd\update-bridge || goto error
powershell -NoProfile -ExecutionPolicy Bypass -File "deploy\make-update-zip.ps1" -Version "%BOOTSTRAP_VERSION%" -TargetOS "linux" -TargetArch "amd64" -Binary "build\linux\build\middleware-v%BOOTSTRAP_VERSION%-linux-amd64" -Out "build\patches\chpp-middleware-v%BOOTSTRAP_VERSION%-linux-amd64-update.zip" || goto error
copy /Y "deploy\chpp-middleware.service" "build\linux\deploy\chpp-middleware.service" >nul
copy /Y "deploy\install-systemd.sh" "build\linux\deploy\install-systemd.sh" >nul
copy /Y "deploy\manage-service.sh" "build\linux\deploy\manage-service.sh" >nul
if exist "README.md" copy /Y "README.md" "build\linux\README.md" >nul
echo Final clean
del /q "build\*.exe~" >nul 2>nul

echo.
echo Build complete:
echo   Windows: %CD%\build\middleware-v%VERSION%-windows-amd64.exe
echo   Debian:  %CD%\build\linux
echo   Patches: %CD%\build\patches
exit /b 0

:error
echo.
echo Build failed.
exit /b 1

