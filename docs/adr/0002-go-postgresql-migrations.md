# ADR-0002: Go, PostgreSQL, pgx, and Embedded Forward-Only Migrations

- Status: Accepted
- Date: 2026-07-24

## Context

The Central Platform needs one transactional database for tenant master data, authentication, telemetry, jobs, and audit records. The approved architecture selects PostgreSQL, `pgx`, and `sqlc`, rejects Prisma for the Go service, and requires one migration owner.

## Decision

`services/platform-api` owns the Central Platform PostgreSQL schema. It connects through `pgx/v5`. Forward-only numbered SQL migrations are embedded in the Platform API release artifact and recorded in `schema_migrations`.

The initial schema stores tenant-owned master data with `organization_id`, enforces Plant latitude/longitude using scalar range constraints, and does not install PostgreSQL extensions. `sqlc` will own application queries when the first master-data API is implemented; migrations remain handwritten reviewed SQL.

## Consequences

- The Platform API fails startup when configuration, database connectivity, or migration application fails.
- Deployments run the same embedded migration set as the released application artifact.
- Prisma and a second migration system must not manage this schema.
- Migration integration tests require an explicitly provided disposable PostgreSQL database; no database service is installed automatically.
