# API Gateway

Go reverse proxy for browser HTTP and WebSocket traffic. Authentication and business authorization remain in `platform-api`.

## Configuration

- `GATEWAY_HTTP_ADDR`: default `127.0.0.1:44440`
- `GATEWAY_PLATFORM_URL`: default `http://127.0.0.1:44441`
- `GATEWAY_ALLOWED_ORIGINS`: comma-separated browser origins, default `http://localhost:8080,http://127.0.0.1:8080`

## Run

```powershell
go run ./cmd/api-gateway
```

- Gateway health: `GET http://localhost:44440/gateway/healthz`
- Platform routes, including `/api/v1/realtime`, are proxied without rewriting.