@echo off
setlocal EnableExtensions
cd /d "%~dp0"

start "auth-service"          cmd /k "cd services\auth-service && go run ./cmd/auth-service"
start "api-gateway"           cmd /k "cd services\api-gateway && go run ./cmd/api-gateway"
start "platform-api"          cmd /k "cd services\platform-api && go run ./cmd/platform-api"
start "modbus-api-middleware" cmd /k "cd modbus-api-middleware && go run ./cmd/middleware"
start "web"                   cmd /k "cd apps\web && npm run dev"

echo All services starting in separate windows.
