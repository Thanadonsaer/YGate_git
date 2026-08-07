-- Heal any database that ran the first version of 000039, which dropped
-- telemetry_ingest_batch.raw_payload outright.
--
-- Migrations are applied by the service at startup, so a database can be
-- ahead of a binary that is still running (rolling deploy, failed rollout,
-- rollback, or a second instance that has not restarted). Those older
-- binaries INSERT raw_payload by name, so against the dropped column every
-- ingest failed with:
--
--   create raw ingest batch: ERROR: column "raw_payload" of relation
--   "telemetry_ingest_batch" does not exist (SQLSTATE 42703)
--
-- Bringing the column back with a '{}' default makes both versions work
-- again: the current code never mentions it and gets the default, older code
-- writes whatever it likes. It does NOT bring the storage cost back, because
-- nothing new writes a real payload and 000039 already blanked the existing
-- ones.
--
-- IF NOT EXISTS so this is a no-op on databases that ran the corrected
-- 000039 and still have the column. Postgres 11+ adds a defaulted column
-- without rewriting the table.

ALTER TABLE telemetry.telemetry_ingest_batch
    ADD COLUMN IF NOT EXISTS raw_payload jsonb NOT NULL DEFAULT '{}'::jsonb;
