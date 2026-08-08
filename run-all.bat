@echo off
setlocal EnableExtensions
cd /d "%~dp0"

REM Stagger the starts: `go run` compiles eagerly and Turbopack forks a worker
REM per core. Launched together they spike Windows commit charge, and an
REM allocation that fails gets baked into apps\web\.next as a cached 500 that
REM survives restarts. Delete apps\web\.next if the web page starts failing.
start "auth-service"          cmd /k "cd services\auth-service && go run ./cmd/auth-service"
timeout /t 8 /nobreak >nul
start "api-gateway"           cmd /k "cd services\api-gateway && go run ./cmd/api-gateway"
timeout /t 8 /nobreak >nul
start "platform-api"          cmd /k "cd services\platform-api && go run ./cmd/platform-api"
timeout /t 8 /nobreak >nul
@REM start "modbus-api-middleware" cmd /k "cd modbus-api-middleware && go run ./cmd/middleware"
start "web"                   cmd /k "cd apps\web && npm run dev"

echo All services starting in separate windows.
