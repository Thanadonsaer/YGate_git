-- Collapse telemetry storage onto raw_register_reading alone, and give the
-- register-mapping rule (raw value -> scale/offset/enabled) exactly one
-- definition in the database.
--
-- Three things happened here over time and all three stored the same physical
-- reading:
--   1. raw_register_reading.register_address_map -- the real, kept store.
--   2. telemetry_reading.data_item_map + telemetry_latest.data_item_map --
--      written only by the v1 dataItemMap ingest path, which no Middleware
--      has sent since delivery moved to schemaVersion 2.0 register maps, and
--      read by nothing: ListLatestPlantTelemetry and ListDeviceTelemetryHistory
--      both already select from raw. Write-only tables.
--   3. telemetry_ingest_batch.raw_payload -- the whole POST/drain body, a
--      second verbatim copy of every reading in the batch. Only payload_hash
--      is ever read back (idempotency conflict detection), never the payload.
--      This is the single largest contributor to database growth: an unbounded,
--      unpartitioned jsonb roughly the same size as the partitioned table it
--      duplicates.
--
-- The mapping rule itself was copy-pasted three times (this view,
-- core/telemetry.go's latestMappedRawTelemetry, and its
-- mappedRawTelemetryHistory), so a change to metadata precedence had to be
-- made in three places or the latest reading and its own history would
-- disagree. It becomes one STABLE function that all three callers share.

-- ---------------------------------------------------------------------
-- Step 1: the mapping rule, once.
-- device-level Register Metadata overrides device-model-level (priority 2
-- over 1); a register with no metadata row at all is passed through
-- unscaled and enabled, same as the view it replaces. Keys are matched both
-- bare ("40072") and prefixed ("reg40072") because auto-onboard writes the
-- prefixed form while the raw payload carries the bare address.
-- ---------------------------------------------------------------------

DROP VIEW IF EXISTS telemetry.raw_telemetry_latest;

CREATE FUNCTION telemetry.mapped_data_items(
    p_organization_id uuid,
    p_plant_id uuid,
    p_device_id uuid,
    p_device_model_id uuid,
    p_register_address_map jsonb
) RETURNS jsonb
LANGUAGE sql
STABLE
PARALLEL SAFE
AS $$
    SELECT COALESCE(
        jsonb_object_agg(
            item.key,
            item.value::double precision * COALESCE(metadata.scale, 1) + COALESCE(metadata.value_offset, 0)
        ) FILTER (WHERE COALESCE(metadata.is_enabled, true)),
        '{}'::jsonb
    )
    FROM jsonb_each_text(p_register_address_map) item
    LEFT JOIN LATERAL (
        SELECT scale, value_offset, is_enabled
        FROM (
            SELECT scale, value_offset, is_enabled, 2 AS priority
            FROM plant.device_register_metadata
            WHERE organization_id = p_organization_id
              AND plant_id = p_plant_id
              AND device_id = p_device_id
              AND (address_key = item.key OR address_key = concat('reg', item.key))
            UNION ALL
            SELECT scale, value_offset, is_enabled, 1 AS priority
            FROM plant.device_model_register_metadata
            WHERE organization_id = p_organization_id
              AND device_model_id = p_device_model_id
              AND (address_key = item.key OR address_key = concat('reg', item.key))
        ) configured
        ORDER BY priority DESC
        LIMIT 1
    ) metadata ON true
$$;

-- ---------------------------------------------------------------------
-- Step 2: "the latest reading per device", once.
-- Split out from raw_telemetry_latest because two callers want it without
-- any register mapping at all: the dashboard only asks "when did this device
-- last report", and answering that through the mapping view made it unnest
-- and metadata-join every register of every device just to throw the result
-- away.
-- ---------------------------------------------------------------------

CREATE VIEW telemetry.raw_register_reading_latest AS
SELECT DISTINCT ON (organization_id, device_id) *
FROM telemetry.raw_register_reading
ORDER BY organization_id, device_id, observed_at DESC, received_at DESC, id DESC;

-- ---------------------------------------------------------------------
-- Step 3: the latest read model = latest row + mapping rule. Mapping is
-- applied to one row per device, never to the whole history.
-- ---------------------------------------------------------------------

CREATE VIEW telemetry.raw_telemetry_latest AS
SELECT
    raw.id AS telemetry_reading_id,
    raw.organization_id,
    raw.plant_id,
    raw.device_id,
    raw.gateway_id,
    raw.observed_at,
    raw.received_at,
    mapped.data_item_map,
    (SELECT count(*)::integer FROM jsonb_object_keys(mapped.data_item_map)) AS parameter_count
FROM telemetry.raw_register_reading_latest raw
JOIN plant.device device
  ON device.organization_id = raw.organization_id
 AND device.plant_id = raw.plant_id
 AND device.id = raw.device_id
CROSS JOIN LATERAL (
    SELECT telemetry.mapped_data_items(
        raw.organization_id, raw.plant_id, raw.device_id,
        device.device_model_id, raw.register_address_map
    ) AS data_item_map
) mapped;

-- ---------------------------------------------------------------------
-- Step 4: drop the write-only duplicate stores.
-- telemetry_latest first: it is the only remaining referent of
-- telemetry_reading rows (by convention since 000034 dropped the FK).
-- DROP TABLE on a partitioned parent drops every partition with it.
-- ---------------------------------------------------------------------

DROP TABLE telemetry.telemetry_latest;
DROP TABLE telemetry.telemetry_reading;

-- ---------------------------------------------------------------------
-- Step 5: stop storing a second verbatim copy of every ingested batch.
-- payload_hash (kept) is what CreateOrGetIngestBatch actually compares to
-- detect an idempotency-key collision on a different payload; raw_payload
-- was never selected.
--
-- The column is emptied and defaulted, NOT dropped. Migrations run at
-- service startup, so between "new binary migrated the database" and "every
-- old binary is gone" both versions talk to this schema at once -- during a
-- rolling deploy, a rollback, or a second instance that has not restarted
-- yet. An older binary still INSERTs raw_payload explicitly, and against a
-- dropped column every single ingest fails with
--   create raw ingest batch: column "raw_payload" of relation
--   "telemetry_ingest_batch" does not exist
-- which takes telemetry down for as long as the mismatch lasts. Keeping the
-- column with a '{}' default costs a few bytes per batch row and reclaims
-- exactly the same space, because the payloads themselves are cleared here.
-- ---------------------------------------------------------------------

UPDATE telemetry.telemetry_ingest_batch
SET raw_payload = '{}'::jsonb
WHERE raw_payload <> '{}'::jsonb;

ALTER TABLE telemetry.telemetry_ingest_batch
    ALTER COLUMN raw_payload SET DEFAULT '{}'::jsonb;

-- ponytail: the column can be dropped for real in a later release, once no
-- binary that writes it can still be running. Until then this is the
-- expand/contract-safe half.
