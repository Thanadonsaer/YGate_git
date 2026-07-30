@echo off
setlocal EnableExtensions
chcp 65001 >nul
cd /d "%~dp0"

:menu
cls
echo CHPP License Tool
echo =================
echo 1^) Generate keypair
echo 2^) Generate customer license token
echo 0^) Exit
echo.
set /p choice=Select: 
if "%choice%"=="1" goto keypair
if "%choice%"=="2" goto token
if "%choice%"=="0" exit /b 0
goto menu

:keypair
go run .\cmd\licensegen -generate-keypair
echo.
echo Keep CHPP_LICENSE_PRIVATE_KEY outside the repo. Put only the public key in license-keys.env.
pause
goto menu

:token
if not defined CHPP_LICENSE_PRIVATE_KEY set /p CHPP_LICENSE_PRIVATE_KEY=Private key ^(base64^): 
if "%CHPP_LICENSE_PRIVATE_KEY%"=="" (echo Missing private key& pause& goto menu)
set /p CUSTOMER=Customer name ^(default CHPP^): 
if "%CUSTOMER%"=="" set "CUSTOMER=CHPP"
set /p MACHINE_ID=Machine ID ^(* for floating^): 
if "%MACHINE_ID%"=="" (echo Missing Machine ID& pause& goto menu)
set /p EXPIRES=Expires YYYY-MM-DD ^(blank = 1 year^): 
echo.
if "%EXPIRES%"=="" (
  go run .\cmd\licensegen -customer "%CUSTOMER%" -machine-id "%MACHINE_ID%"
) else (
  go run .\cmd\licensegen -customer "%CUSTOMER%" -machine-id "%MACHINE_ID%" -expires "%EXPIRES%"
)
echo.
pause
goto menu
