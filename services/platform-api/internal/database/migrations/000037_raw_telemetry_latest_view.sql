-- Raw is the telemetry source of truth. This view is a calculated latest
-- read model and must never be treated as an independent history store.
CREATE OR REPLACE VIEW telemetry.raw_telemetry_latest AS
WITH latest_raw AS (
    SELECT DISTINCT ON (organization_id, device_id) *
    FROM telemetry.raw_register_reading
    ORDER BY organization_id, device_id, observed_at DESC, received_at DESC, id DESC
)
SELECT
    raw.id AS telemetry_reading_id,
    raw.organization_id,
    raw.plant_id,
    raw.device_id,
    raw.gateway_id,
    raw.observed_at,
    raw.received_at,
    COALESCE(
        jsonb_object_agg(
            item.key,
            item.value::double precision * COALESCE(metadata.scale, 1) + COALESCE(metadata.value_offset, 0)
        ) FILTER (WHERE COALESCE(metadata.is_enabled, true)),
        '{}'::jsonb
    ) AS data_item_map,
    count(*) FILTER (WHERE COALESCE(metadata.is_enabled, true))::integer AS parameter_count
FROM latest_raw raw
JOIN plant.device device
  ON device.organization_id = raw.organization_id
 AND device.plant_id = raw.plant_id
 AND device.id = raw.device_id
CROSS JOIN LATERAL jsonb_each_text(raw.register_address_map) item
LEFT JOIN LATERAL (
    SELECT scale, value_offset, is_enabled
    FROM (
        SELECT scale, value_offset, is_enabled, 2 AS priority
        FROM plant.device_register_metadata
        WHERE organization_id = raw.organization_id
          AND plant_id = raw.plant_id
          AND device_id = raw.device_id
          AND (address_key = item.key OR address_key = concat('reg', item.key))
        UNION ALL
        SELECT scale, value_offset, is_enabled, 1 AS priority
        FROM plant.device_model_register_metadata
        WHERE organization_id = raw.organization_id
          AND device_model_id = device.device_model_id
          AND (address_key = item.key OR address_key = concat('reg', item.key))
    ) configured
    ORDER BY priority DESC
    LIMIT 1
) metadata ON true
GROUP BY raw.id, raw.organization_id, raw.plant_id, raw.device_id,
         raw.gateway_id, raw.observed_at, raw.received_at;