# ADR-0003: Go API Gateway and authenticated WebSocket edge

- Status: Accepted
- Date: 2026-07-24

## Context

The browser must run on port 8080, enter the backend through a Go API Gateway on port 44440, and receive realtime updates over WebSocket. The existing `platform-api` owns authentication and business authorization. The normal Middleware payload remains the versioned `dataItemMap` compatibility input and is not changed by this browser transport decision.

## Decision

- `apps/web` listens on `8080`.
- `services/api-gateway` listens on `44440` and proxies HTTP and WebSocket traffic to `platform-api`.
- `services/platform-api` defaults to internal port `44441`.
- The Gateway owns CORS, request IDs, security response headers, and reverse proxying only. It does not make authorization decisions or contain business logic.
- `GET /api/v1/realtime` requires the existing secure browser session. Platform API validates the WebSocket Origin allowlist before upgrading.
- The initial realtime contract emits connection readiness and heartbeat frames. Telemetry messages will be added only with the Phase 2 ingestion/event contract so no invented measurements replace `dataItemMap`.

## Consequences

The browser has one backend entry point and WebSocket upgrades remain compatible with the same cookie session. Local development needs three processes, but no external gateway product or message broker is introduced. Production TLS termination remains the responsibility of the organization's existing reverse proxy or load balancer.