-- Convert telemetry.telemetry_reading and telemetry.raw_register_reading
-- to native Postgres RANGE partitioning by observed_at, so future growth
-- doesn't degrade query/vacuum performance and future retention (dropping
-- old partitions) becomes a cheap DROP TABLE instead of a slow DELETE.
-- See docs/superpowers/specs/2026-08-05-telemetry-partitioning-design.md.
--
-- Runs after 000033_schema_namespacing.sql, so every reference here is
-- schema-qualified the same way that migration qualifies its own tables.
--
-- Two tables are deliberately excluded, for two distinct reasons:
--   - telemetry_latest is a keyed snapshot table (one row per device,
--     updated in place), not an append-only time series.
--   - telemetry_ingest_batch's idempotency dedup relies on `received_at`
--     being a DB-generated `now()` value with no caller-supplied stable
--     equivalent (unlike telemetry_reading/raw_register_reading's
--     `observed_at`, which is a caller-supplied event-time value, stable
--     across retries of the same physical reading). Postgres requires
--     every UNIQUE constraint on a partitioned table to include the
--     partition key column; widening telemetry_ingest_batch's idempotency
--     constraint to include `received_at` would make its idempotency
--     check fire almost never (every retry gets a fresh timestamp),
--     silently breaking the exact guarantee that constraint exists for.
--     telemetry_ingest_batch is lower-volume than the two tables below
--     anyway (one row per middleware upload batch, not per reading).
--
-- Postgres requires every PRIMARY KEY/UNIQUE constraint on a partitioned
-- table to include the partition key column. This forces two changes on
-- the two tables that are partitioned:
--   1. telemetry_latest's incoming FK to telemetry_reading(id) is dropped
--      rather than widened (explicit decision, time-boxed): referential
--      integrity here becomes an application-level convention.
--      hard_delete.go already deletes in explicit dependency order, not
--      by relying on ON DELETE RESTRICT to catch mistakes, so this
--      doesn't change actual delete-cascade behavior.
--   2. Each table's dedup UNIQUE constraint widens to include
--      observed_at. Since observed_at is the same physical-reading
--      identity value on every retry (unlike telemetry_ingest_batch's
--      received_at), this preserves the dedup guarantee correctly.
--
-- No existing table can be ALTERed into a partitioned table directly, so
-- each table is: renamed aside, recreated as partitioned with its
-- partitions, backfilled from the renamed original (near-zero rows today,
-- not a real backfill), then the original is dropped.
-- telemetry_reading_batch_fk and raw_register_reading_batch_fk (both
-- referencing telemetry_ingest_batch) are untouched — that table stays
-- unpartitioned, so its existing FK targets remain valid as-is.

-- ---------------------------------------------------------------------
-- Step 1: drop the one incoming FK that blocks partitioning
-- telemetry_reading. Default Postgres naming for a column-level
-- REFERENCES with no explicit CONSTRAINT name is <table>_<column>_fkey.
-- ---------------------------------------------------------------------

ALTER TABLE telemetry.telemetry_latest DROP CONSTRAINT telemetry_latest_telemetry_reading_id_fkey;

-- ---------------------------------------------------------------------
-- Step 2: telemetry_reading, partitioned by observed_at
-- ---------------------------------------------------------------------

ALTER TABLE telemetry.telemetry_reading RENAME TO telemetry_reading_old;
-- PK/UNIQUE constraints back schema-wide-unique index names (unlike CHECK/FK
-- constraint names, which are only unique per-table) — rename the two that
-- would otherwise collide with the new table's constraints of the same name.
ALTER TABLE telemetry.telemetry_reading_old RENAME CONSTRAINT telemetry_reading_pkey TO telemetry_reading_old_pkey;
ALTER TABLE telemetry.telemetry_reading_old RENAME CONSTRAINT telemetry_reading_client_external_unique TO telemetry_reading_old_client_external_unique;
-- Plain (non-constraint-backed) indexes also occupy schema-wide-unique names.
ALTER INDEX telemetry.telemetry_reading_plant_time_idx RENAME TO telemetry_reading_old_plant_time_idx;
ALTER INDEX telemetry.telemetry_reading_device_time_idx RENAME TO telemetry_reading_old_device_time_idx;

CREATE TABLE telemetry.telemetry_reading (
    id uuid NOT NULL,
    organization_id uuid NOT NULL,
    plant_id uuid NOT NULL,
    device_id uuid NOT NULL,
    middleware_client_id uuid NOT NULL,
    ingest_batch_id uuid NOT NULL,
    gateway_id text NOT NULL,
    external_key text NOT NULL,
    observed_at timestamptz NOT NULL,
    received_at timestamptz NOT NULL DEFAULT now(),
    data_item_map jsonb NOT NULL,
    parameter_count integer NOT NULL,
    CONSTRAINT telemetry_reading_pkey PRIMARY KEY (id, observed_at),
    CONSTRAINT telemetry_reading_plant_fk FOREIGN KEY (organization_id, plant_id)
        REFERENCES plant.plant(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT telemetry_reading_device_fk FOREIGN KEY (organization_id, plant_id, device_id)
        REFERENCES plant.device(organization_id, plant_id, id) ON DELETE RESTRICT,
    CONSTRAINT telemetry_reading_client_fk FOREIGN KEY (organization_id, middleware_client_id)
        REFERENCES auth.middleware_client(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT telemetry_reading_batch_fk FOREIGN KEY (organization_id, ingest_batch_id)
        REFERENCES telemetry.telemetry_ingest_batch(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT telemetry_reading_gateway_length CHECK (length(btrim(gateway_id)) BETWEEN 1 AND 200),
    CONSTRAINT telemetry_reading_external_key_length CHECK (length(external_key) BETWEEN 1 AND 500),
    CONSTRAINT telemetry_reading_map_object CHECK (jsonb_typeof(data_item_map) = 'object'),
    CONSTRAINT telemetry_reading_parameter_count CHECK (parameter_count >= 0),
    CONSTRAINT telemetry_reading_client_external_unique UNIQUE (middleware_client_id, external_key, observed_at)
) PARTITION BY RANGE (observed_at);

CREATE TABLE telemetry.telemetry_reading_2026_06 PARTITION OF telemetry.telemetry_reading
    FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');
CREATE TABLE telemetry.telemetry_reading_2026_07 PARTITION OF telemetry.telemetry_reading
    FOR VALUES FROM ('2026-07-01') TO ('2026-08-01');
CREATE TABLE telemetry.telemetry_reading_2026_08 PARTITION OF telemetry.telemetry_reading
    FOR VALUES FROM ('2026-08-01') TO ('2026-09-01');
CREATE TABLE telemetry.telemetry_reading_2026_09 PARTITION OF telemetry.telemetry_reading
    FOR VALUES FROM ('2026-09-01') TO ('2026-10-01');
CREATE TABLE telemetry.telemetry_reading_2026_10 PARTITION OF telemetry.telemetry_reading
    FOR VALUES FROM ('2026-10-01') TO ('2026-11-01');
CREATE TABLE telemetry.telemetry_reading_2026_11 PARTITION OF telemetry.telemetry_reading
    FOR VALUES FROM ('2026-11-01') TO ('2026-12-01');
CREATE TABLE telemetry.telemetry_reading_2027_02 PARTITION OF telemetry.telemetry_reading
    FOR VALUES FROM ('2026-12-01') TO ('2027-03-01');
CREATE TABLE telemetry.telemetry_reading_default PARTITION OF telemetry.telemetry_reading DEFAULT;

INSERT INTO telemetry.telemetry_reading (
    id, organization_id, plant_id, device_id, middleware_client_id,
    ingest_batch_id, gateway_id, external_key, observed_at, received_at,
    data_item_map, parameter_count
)
SELECT id, organization_id, plant_id, device_id, middleware_client_id,
       ingest_batch_id, gateway_id, external_key, observed_at, received_at,
       data_item_map, parameter_count
FROM telemetry.telemetry_reading_old;

CREATE INDEX telemetry_reading_plant_time_idx ON telemetry.telemetry_reading (organization_id, plant_id, observed_at DESC);
CREATE INDEX telemetry_reading_device_time_idx ON telemetry.telemetry_reading (organization_id, device_id, observed_at DESC);

DROP TABLE telemetry.telemetry_reading_old;

-- ---------------------------------------------------------------------
-- Step 3: raw_register_reading, partitioned by observed_at
-- ---------------------------------------------------------------------

ALTER TABLE telemetry.raw_register_reading RENAME TO raw_register_reading_old;
ALTER TABLE telemetry.raw_register_reading_old RENAME CONSTRAINT raw_register_reading_pkey TO raw_register_reading_old_pkey;
ALTER TABLE telemetry.raw_register_reading_old RENAME CONSTRAINT raw_register_reading_client_external_unique TO raw_register_reading_old_client_external_unique;
ALTER INDEX telemetry.raw_register_reading_plant_time_idx RENAME TO raw_register_reading_old_plant_time_idx;
ALTER INDEX telemetry.raw_register_reading_device_time_idx RENAME TO raw_register_reading_old_device_time_idx;

CREATE TABLE telemetry.raw_register_reading (
    id uuid NOT NULL,
    organization_id uuid NOT NULL,
    plant_id uuid NOT NULL,
    device_id uuid NOT NULL,
    middleware_client_id uuid NOT NULL,
    ingest_batch_id uuid NOT NULL,
    gateway_id text NOT NULL,
    external_key text NOT NULL,
    observed_at timestamptz NOT NULL,
    received_at timestamptz NOT NULL DEFAULT now(),
    register_address_map jsonb NOT NULL,
    parameter_count integer NOT NULL,
    CONSTRAINT raw_register_reading_pkey PRIMARY KEY (id, observed_at),
    CONSTRAINT raw_register_reading_plant_fk FOREIGN KEY (organization_id, plant_id)
        REFERENCES plant.plant(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT raw_register_reading_device_fk FOREIGN KEY (organization_id, plant_id, device_id)
        REFERENCES plant.device(organization_id, plant_id, id) ON DELETE RESTRICT,
    CONSTRAINT raw_register_reading_client_fk FOREIGN KEY (organization_id, middleware_client_id)
        REFERENCES auth.middleware_client(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT raw_register_reading_batch_fk FOREIGN KEY (organization_id, ingest_batch_id)
        REFERENCES telemetry.telemetry_ingest_batch(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT raw_register_reading_gateway_length CHECK (length(btrim(gateway_id)) BETWEEN 1 AND 200),
    CONSTRAINT raw_register_reading_external_key_length CHECK (length(external_key) BETWEEN 1 AND 500),
    CONSTRAINT raw_register_reading_map_object CHECK (jsonb_typeof(register_address_map) = 'object'),
    CONSTRAINT raw_register_reading_parameter_count CHECK (parameter_count BETWEEN 1 AND 5000),
    CONSTRAINT raw_register_reading_client_external_unique UNIQUE (middleware_client_id, external_key, observed_at)
) PARTITION BY RANGE (observed_at);

CREATE TABLE telemetry.raw_register_reading_2026_06 PARTITION OF telemetry.raw_register_reading
    FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');
CREATE TABLE telemetry.raw_register_reading_2026_07 PARTITION OF telemetry.raw_register_reading
    FOR VALUES FROM ('2026-07-01') TO ('2026-08-01');
CREATE TABLE telemetry.raw_register_reading_2026_08 PARTITION OF telemetry.raw_register_reading
    FOR VALUES FROM ('2026-08-01') TO ('2026-09-01');
CREATE TABLE telemetry.raw_register_reading_2026_09 PARTITION OF telemetry.raw_register_reading
    FOR VALUES FROM ('2026-09-01') TO ('2026-10-01');
CREATE TABLE telemetry.raw_register_reading_2026_10 PARTITION OF telemetry.raw_register_reading
    FOR VALUES FROM ('2026-10-01') TO ('2026-11-01');
CREATE TABLE telemetry.raw_register_reading_2026_11 PARTITION OF telemetry.raw_register_reading
    FOR VALUES FROM ('2026-11-01') TO ('2026-12-01');
CREATE TABLE telemetry.raw_register_reading_2027_02 PARTITION OF telemetry.raw_register_reading
    FOR VALUES FROM ('2026-12-01') TO ('2027-03-01');
CREATE TABLE telemetry.raw_register_reading_default PARTITION OF telemetry.raw_register_reading DEFAULT;

INSERT INTO telemetry.raw_register_reading (
    id, organization_id, plant_id, device_id, middleware_client_id,
    ingest_batch_id, gateway_id, external_key, observed_at, received_at,
    register_address_map, parameter_count
)
SELECT id, organization_id, plant_id, device_id, middleware_client_id,
       ingest_batch_id, gateway_id, external_key, observed_at, received_at,
       register_address_map, parameter_count
FROM telemetry.raw_register_reading_old;

CREATE INDEX raw_register_reading_plant_time_idx ON telemetry.raw_register_reading (organization_id, plant_id, observed_at DESC);
CREATE INDEX raw_register_reading_device_time_idx ON telemetry.raw_register_reading (organization_id, device_id, observed_at DESC);

DROP TABLE telemetry.raw_register_reading_old;
