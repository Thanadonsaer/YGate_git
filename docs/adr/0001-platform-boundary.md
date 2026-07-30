# ADR-0001: Separate Central Platform from Site Middleware

- Status: Accepted
- Date: 2026-07-24

## Context

The existing `modbus-api-middleware` is a Site/Edge deployable. It polls Modbus devices, normalizes Address Configuration values, stores a local SQLite outbox, and sends batches containing `dataItemMap`. The new Solar SCADA Web Platform is the central receiver and must have an independent lifecycle, persistence model, security boundary, and release artifact.

## Decision

Create the Central Platform as separate deployables under `services/` and `apps/`. Start with `services/platform-api` as the Go modular-monolith API. Do not import Middleware internal packages into the Platform.

The Middleware contract remains compatibility input: normal payloads contain normalized `dataItemMap`; detailed Poll Log measurements such as raw value, function code, and register address are not required fields.

## Consequences

- Middleware releases remain independent from Platform releases.
- Cross-boundary behavior is defined by versioned OpenAPI contracts and contract tests.
- PostgreSQL, authentication, ingestion, workers, and the Next.js application are added in later approved milestones.
- No additional infrastructure service is introduced by this decision.
