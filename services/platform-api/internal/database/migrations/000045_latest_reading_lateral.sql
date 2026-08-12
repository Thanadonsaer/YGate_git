-- "Latest reading per device" stops scanning the whole telemetry history.
--
-- 000039 defined it as
--
--     SELECT DISTINCT ON (organization_id, device_id) *
--     FROM telemetry.raw_register_reading
--     ORDER BY organization_id, device_id, observed_at DESC, ...
--
-- and left a ponytail note in queries/dashboard.sql that this "walks the whole
-- device-time index once per dashboard load ... if it shows up in slow-query
-- logs, replace with a correlated LATERAL". It has: opening a Plant, the SCADA
-- Builder or the SCADA Viewer is slow, and gets slower every week.
--
-- Two things make the DISTINCT ON worse than that note assumed:
--
--   1. raw_register_reading is partitioned by observed_at (000034), and the
--      device-time index exists per partition, not across the parent. A
--      DISTINCT ON spanning all partitions therefore cannot be answered by an
--      ordered index scan -- Postgres appends every partition and sorts the
--      lot. Cost is O(all readings ever ingested), for one row per device.
--   2. ListLatestPlantTelemetry filters `plant_id = $2`, but plant_id is not in
--      the DISTINCT ON key, so that predicate cannot be pushed down. Opening
--      one plant paid for every device in the organization.
--
-- Driving from plant.device instead inverts it: one index descent per device
-- per partition (LIMIT 1 on the device-time index), and
-- organization_id/plant_id/device_id are now plain columns of `device`, so
-- callers' WHERE clauses push straight into the device scan and a single-plant
-- page touches only that plant's devices.
--
-- Same view name, same columns, same one-row-per-device meaning, so
-- raw_telemetry_latest and both callers are unchanged.

CREATE OR REPLACE VIEW telemetry.raw_register_reading_latest AS
SELECT
    latest.id,
    device.organization_id,
    device.plant_id,
    device.id AS device_id,
    latest.middleware_client_id,
    latest.ingest_batch_id,
    latest.gateway_id,
    latest.external_key,
    latest.observed_at,
    latest.received_at,
    latest.register_address_map,
    latest.parameter_count
FROM plant.device device
CROSS JOIN LATERAL (
    SELECT raw.id, raw.middleware_client_id, raw.ingest_batch_id, raw.gateway_id,
           raw.external_key, raw.observed_at, raw.received_at,
           raw.register_address_map, raw.parameter_count
    FROM telemetry.raw_register_reading raw
    WHERE raw.organization_id = device.organization_id
      AND raw.plant_id = device.plant_id
      AND raw.device_id = device.id
    ORDER BY raw.observed_at DESC, raw.received_at DESC, raw.id DESC
    LIMIT 1
) latest;

-- The LATERAL's ORDER BY is (observed_at, received_at, id) DESC within one
-- device. The existing raw_register_reading_device_time_idx stops at
-- observed_at, so a device with several readings sharing one observed_at makes
-- Postgres sort that tie group. Widening the index keeps LIMIT 1 a pure
-- descent even then, and still serves the (organization_id, device_id) lookups
-- the old index served, so the old one is dropped rather than kept alongside.
CREATE INDEX IF NOT EXISTS raw_register_reading_device_latest_idx
    ON telemetry.raw_register_reading (organization_id, device_id, observed_at DESC, received_at DESC, id DESC);
DROP INDEX IF EXISTS telemetry.raw_register_reading_device_time_idx;
