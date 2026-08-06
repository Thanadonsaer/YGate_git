-- Index audit (sub-project 3/4, time-boxed). Scope: FK columns with no
-- covering index, filtered to the ones with verified real query traffic
-- (hard_delete.go's cascades), not a blind index of every FK gap found —
-- every index has a write-cost tradeoff.
--
-- Found via direct catalog introspection (pg_constraint/pg_index) against
-- the real local database: ~25 FK columns across 7 schemas initially
-- looked uncovered. Most are low-traffic admin/audit columns on small
-- tables (scada_screen.updated_by, middleware_patch.uploaded_by,
-- site_setting.updated_by, alarm_rule.notify_role_id,
-- email_verification_token.user_id) — skipped, not worth the write cost
-- given the time-box; a follow-up pass can revisit if they show up in
-- real slow-query logs.
--
-- Two more (auth.user_role, telemetry.telemetry_latest) turned out to
-- already have adequate coverage once checked against actual existing
-- index definitions, not just an FK-column-order-exact-match heuristic:
-- user_role_user_scope_idx already covers (user_id, organization_id, ...)
-- — reversed leading-column order from the FK declaration, but Postgres
-- can use both columns as equality conditions regardless of order, so it
-- serves the permission-check hot path fine. telemetry_latest_plant_time_idx
-- already covers (organization_id, plant_id, ...), serving hard_delete.go's
-- plant-level cascade filter. Neither needed a new index — verified by
-- reading `pg_indexes`, not assumed.
--
-- The 3 added here were each matched against an actual WHERE clause in
-- the codebase: hard_delete.go deletes by exactly
-- `WHERE organization_id=$1 AND middleware_client_id=$2` when
-- hard-deleting a middleware client
-- (internal/core/hard_delete.go:187-189), and none of these three tables
-- had any index covering that filter (each has plant/device-time indexes
-- and uniqueness constraints, but none on middleware_client_id alone).
--
-- Created on the partitioned tables' parent relation, which Postgres
-- automatically propagates to every existing and future partition — no
-- per-partition CREATE INDEX needed.

CREATE INDEX telemetry_reading_client_idx ON telemetry.telemetry_reading (organization_id, middleware_client_id);
CREATE INDEX raw_register_reading_client_idx ON telemetry.raw_register_reading (organization_id, middleware_client_id);
CREATE INDEX telemetry_ingest_batch_client_idx ON telemetry.telemetry_ingest_batch (organization_id, middleware_client_id);
